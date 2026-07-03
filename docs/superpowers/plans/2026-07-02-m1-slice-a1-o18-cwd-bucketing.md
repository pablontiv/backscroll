# A1. O18 — Workspace Bucketing by CWD — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [x]) syntax for tracking.

## Goal

Plumb the session `cwd` through the complete indexing pipeline so sessions are correctly bucketed by project identity rather than resolving to `unknown`. Handle cross-host path equivalence (`/home/shared` vs `/Users/Shared` roots on a synced index) via the project registry, not raw string matching.

## Architecture

The pipeline already extracts cwd from session records in readers (ClaudeReader extracts it via `sync.IterateJSONLFile`), stores it in ParsedFile, and attempts identification in sync_helpers.go. The missing piece is robust cross-host path equivalence matching in `projects.Identify()`. Today it performs exact string matching; it must instead normalize equivalent roots before comparison.

Add a helper function `normalizePathForEquivalence()` that maps cross-host root aliases (from the project registry) back to a canonical form, then use this in Identify's subpath matching to handle synced indexes where `/home/shared` and `/Users/Shared` refer to the same project.

## Tech Stack

Go (stdlib path/filepath, strings); existing projects package (ProjectRegistry, ProjectConfig); no new external dependencies.

## Global Constraints

- Pure Go, no CGO.
- `just check` (gofmt + go vet) and `just test` must pass after each task.
- Per-package coverage ≥85% enforced pre-push via pkcov (`.coverage-floors.toml`).
- Schema migrations: append-only (new version blocks only; never edit old ones).
- Conventional commits; all code/comments in English.
- DSN pragmas MUST use `_pragma=name(value)` form in modernc.org/sqlite.

---

## Task 1: Add cross-host equivalence helper to projects package

**Files**
- Modify: `internal/projects/projects.go` (add new exported functions after line 79, before Identify)
- Test: existing `internal/projects/projects_test.go`

**Interfaces**
- Consumes: ProjectRegistry (existing)
- Produces: new exported function `NormalizeRootEquivalence(cwd string, registry ProjectRegistry) string` — returns the cwd with known equivalent roots mapped to their canonical form

**Steps**

- [x] Write failing test: add test case `TestNormalizeRootEquivalence_CrossHost` to projects_test.go that:
  - Creates a ProjectRegistry with a project root `/home/shared/myproject` (ONLY this root; no /Users/Shared)
  - Calls `NormalizeRootEquivalence("/Users/Shared/myproject/src", registry)` 
  - Expects exactly `/home/shared/myproject/src` (cwd remapped to canonical root with subpath preserved)
  - Run: `go test -run TestNormalizeRootEquivalence ./internal/projects/` — should FAIL (function does not exist)

```go
func TestNormalizeRootEquivalence_CrossHost(t *testing.T) {
	reg := ProjectRegistry{
		Projects: []ProjectConfig{
			{
				ID:    "myproj",
				Roots: []string{"/home/shared/myproject"},  // Only ONE root; /Users/Shared is cross-host equivalent
			},
		},
	}

	result := NormalizeRootEquivalence("/Users/Shared/myproject/src", reg)
	expected := filepath.Join("/home/shared/myproject", "src")
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}
```

- [x] Minimal implementation in projects.go (correct algorithm; detects cross-host by finding root as contiguous suffix):

```go
// NormalizeRootEquivalence maps equivalent roots (from cross-host syncs like
// /home/shared vs /Users/Shared) to a canonical form using the project registry.
// It finds the longest suffix of a canonical root that appears as a contiguous
// sequence in cwd's components. If found, cwd is remapped to use that canonical root.
// If no equivalent is found, cwd is returned unchanged.
func NormalizeRootEquivalence(cwd string, registry ProjectRegistry) string {
	if cwd == "" {
		return cwd
	}

	for _, p := range registry.Projects {
		for _, canonicalRoot := range p.Roots {
			if isCrossHostEquivalent(cwd, canonicalRoot) {
				return remapPath(cwd, canonicalRoot)
			}
		}
	}

	return cwd
}

// isCrossHostEquivalent returns true if cwd and canonicalRoot refer to the same
// logical path with different host/mount prefixes.
// It checks if a suffix of canonicalRoot's path components appears as a contiguous
// sequence in cwd's components, using case-insensitive comparison.
func isCrossHostEquivalent(cwd, canonicalRoot string) bool {
	cwdParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(cwd)), "/"), "/")
	rootParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(canonicalRoot)), "/"), "/")

	// Require at least 3 components in cwd and 2 in root
	if len(cwdParts) < 3 || len(rootParts) < 2 {
		return false
	}

	// Try tail lengths from longest to shortest (down to 2 components minimum)
	for tailLen := len(rootParts); tailLen >= 2; tailLen-- {
		tail := rootParts[len(rootParts)-tailLen:]

		// Check if cwd_parts contains tail as a contiguous subsequence
		for i := 0; i <= len(cwdParts)-len(tail); i++ {
			if slicesEqualFold(cwdParts[i:i+len(tail)], tail) {
				return true
			}
		}
	}

	return false
}

// remapPath rewrites cwd to use canonicalRoot, preserving any trailing subpath.
// Assumes isCrossHostEquivalent(cwd, canonicalRoot) returned true.
func remapPath(cwd, canonicalRoot string) string {
	cwdParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(cwd)), "/"), "/")
	rootParts := strings.Split(strings.TrimPrefix(filepath.ToSlash(filepath.Clean(canonicalRoot)), "/"), "/")

	// Find the matching tail and its position in cwdParts
	for tailLen := len(rootParts); tailLen >= 2; tailLen-- {
		tail := rootParts[len(rootParts)-tailLen:]

		for i := 0; i <= len(cwdParts)-len(tail); i++ {
			if slicesEqualFold(cwdParts[i:i+len(tail)], tail) {
				// Found tail at position i; extract rest (subpath after tail)
				rest := cwdParts[i+len(tail):]
				// Reconstruct: canonical root + rest
				parts := append(rootParts, rest...)
				return filepath.Join(parts...)
			}
		}
	}

	return cwd
}

// slicesEqualFold reports whether a and b are equal under Unicode case-folding.
func slicesEqualFold(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}
```

Hand-trace of example:
- cwd = "/Users/Shared/myproject/src"
- canonicalRoot = "/home/shared/myproject"
- cwdParts = ["Users", "Shared", "myproject", "src"]
- rootParts = ["home", "shared", "myproject"]
- tailLen=3: tail=["home","shared","myproject"], no contiguous match in cwdParts
- tailLen=2: tail=["shared","myproject"]
  - i=1: cwdParts[1:3]=["Shared","myproject"] vs tail — "Shared".EqualFold("shared")=true, "myproject".EqualFold("myproject")=true → MATCH
  - remapPath: rest=cwdParts[1+2:]=["src"], result=filepath.Join(["home","shared","myproject","src"])="/home/shared/myproject/src" ✓

- [x] Run test: `go test -run TestNormalizeRootEquivalence ./internal/projects/` — should PASS

- [x] Add second test case for no-match scenario:

```go
func TestNormalizeRootEquivalence_NoMatch(t *testing.T) {
	reg := ProjectRegistry{
		Projects: []ProjectConfig{
			{
				ID:    "other",
				Roots: []string{"/home/other/project"},
			},
		},
	}

	result := NormalizeRootEquivalence("/Users/Shared/myproject/src", reg)
	// Expect unchanged since there is no equivalent
	if result != "/Users/Shared/myproject/src" {
		t.Errorf("expected unchanged path, got %s", result)
	}
}
```

- [x] Run tests: `go test -run TestNormalizeRootEquivalence ./internal/projects/` — should PASS both

- [x] Run full projects package tests: `go test ./internal/projects/` — should PASS; check coverage is ≥85% via `go test -cover ./internal/projects/`

- [x] Commit: 
```bash
git add internal/projects/projects.go internal/projects/projects_test.go
git commit -m "feat(projects): add NormalizeRootEquivalence for cross-host path mapping"
```

---

## Task 2: Integrate normalization into Identify

**Files**
- Modify: `internal/projects/projects.go` (update Identify function at line 82)
- Test: existing `internal/projects/projects_test.go`

**Interfaces**
- Consumes: ProjectRegistry, cwd string
- Produces: Identification struct with normalized cross-host paths resolved correctly

**Steps**

- [x] Write failing test: add test case `TestIdentify_CrossHostEquivalence` to projects_test.go that:
  - Creates a ProjectRegistry with root `/home/shared/myproject` ONLY (simulating Linux-only registry)
  - Calls `Identify("/Users/Shared/myproject/src", registry)` (macOS cwd, synced index from Linux)
  - Expects ProjectID == "myproj", Confidence != ConfidenceUnknown
  - Run: `go test -run TestIdentify_CrossHostEquivalence ./internal/projects/` — should FAIL (will resolve to "unknown")

```go
func TestIdentify_CrossHostEquivalence(t *testing.T) {
	reg := ProjectRegistry{
		Projects: []ProjectConfig{
			{
				ID:    "myproj",
				Roots: []string{"/home/shared/myproject"},  // Only Linux root; no macOS equivalent
			},
		},
	}

	result := Identify("/Users/Shared/myproject/src", reg)
	if result.ProjectID != "myproj" {
		t.Errorf("expected ProjectID 'myproj', got %q", result.ProjectID)
	}
	if result.Confidence == ConfidenceUnknown {
		t.Errorf("expected non-unknown confidence, got %s", result.Confidence)
	}
}
```

- [x] Modify Identify function to normalize cwd before matching. Replace line 82 onwards with:

```go
// Identify resolves the canonical project for cwd.
// Resolution order: local hint → exact root → worktree pattern → subpath → truncated suffix → unknown.
// Paths are normalized for cross-host equivalence (e.g., /home/shared vs /Users/Shared roots).
func Identify(cwd string, registry ProjectRegistry) Identification {
	if hint := LoadLocalHint(cwd); hint != nil {
		return Identification{ProjectID: hint.ProjectID, Confidence: ConfidenceHint}
	}

	// Normalize cwd for cross-host equivalence
	normalizedCwd := NormalizeRootEquivalence(cwd, registry)

	// 1. Exact root match (normalizedCwd == root itself).
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			if normalizedCwd == root {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceExact}
			}
		}
	}

	// 2. Worktree pattern match — checked before subpath so worktrees get "pattern" confidence.
	for _, p := range registry.Projects {
		for _, pattern := range p.WorktreePatterns {
			if matched, _ := filepath.Match(pattern, normalizedCwd); matched {
				return Identification{ProjectID: p.ID, Confidence: ConfidencePattern}
			}
		}
	}

	// 3. Subpath under a known root.
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			if strings.HasPrefix(normalizedCwd, root+string(filepath.Separator)) {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceExact}
			}
		}
	}

	// Truncated path: normalizedCwd suffix matches a known root (leading path stripped).
	cwdClean := strings.TrimPrefix(normalizedCwd, string(filepath.Separator))
	for _, p := range registry.Projects {
		for _, root := range p.Roots {
			rootClean := strings.TrimPrefix(root, string(filepath.Separator))
			if strings.HasSuffix(rootClean, cwdClean) || strings.HasSuffix(cwdClean, rootClean) {
				return Identification{ProjectID: p.ID, Confidence: ConfidenceTruncated}
			}
		}
	}

	return Identification{ProjectID: "unknown", Confidence: ConfidenceUnknown}
}
```

- [x] Run test: `go test -run TestIdentify_CrossHostEquivalence ./internal/projects/` — should PASS

- [x] Run all projects tests: `go test ./internal/projects/` — should PASS; check coverage ≥85%

- [x] Commit:
```bash
git add internal/projects/projects.go
git commit -m "fix(projects): normalize cwd for cross-host equivalence in Identify"
```

---

## Task 3: Verify pipeline integration in sync_helpers

**Files**
- Review (no changes): `cmd/backscroll/sync_helpers.go` (lines 84-89)
- Test: `cmd/backscroll/main_test.go` (add integration test)

**Interfaces**
- Consumes: ParsedFile with cwd, ProjectRegistry
- Produces: IndexedFile with correct Project field

**Steps**

- [x] Review the current flow in sync_helpers.go (lines 84-89). Confirm that:
  - Line 85: `identPath := pf.Cwd` uses ParsedFile's cwd
  - Line 89: `projects.Identify(identPath, registry)` now benefits from NormalizeRootEquivalence via Identify
  - Line 110: `Project: ident.ProjectID` stores the resolved project ID

- [x] Write failing integration test in cmd/backscroll/main_test.go (or a new file if tests are organized differently):

```go
func TestIntegration_SyncWithCrosshostEquivalence(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)

	// Create a temporary database path
	dbPath := filepath.Join(tempDir, "test.db")

	// Create a test session JSONL file with cwd set
	sessionDir := filepath.Join(tempDir, ".claude", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	// Write a Claude JSONL session with /Users/Shared/project cwd
	sessionFile := filepath.Join(sessionDir, "test_session.jsonl")
	sessionJSON := `{"type":"user","cwd":"/Users/Shared/myproject/src","timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"hello"}}`
	if err := os.WriteFile(sessionFile, []byte(sessionJSON+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write session: %v", err)
	}

	// Create a projects registry with ONLY /home/shared root (simulating Linux registry)
	projectsPath := filepath.Join(tempDir, ".config", "backscroll")
	if err := os.MkdirAll(projectsPath, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	projectsTOML := `
[[projects]]
id = "myproj"
roots = ["/home/shared/myproject"]
`
	if err := os.WriteFile(filepath.Join(projectsPath, "projects.toml"), []byte(projectsTOML), 0o644); err != nil {
		t.Fatalf("failed to write projects.toml: %v", err)
	}

	// Run maybeAutoSync
	cfg := &config.Config{
		DatabasePath: dbPath,
		SessionDirs:  []string{filepath.Join(tempDir, ".claude", "sessions")},
	}
	if err := maybeAutoSync(cfg); err != nil {
		t.Fatalf("maybeAutoSync failed: %v", err)
	}

	// Verify that the session was indexed with project "myproj"
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Query for the indexed session
	results, err := db.Search("hello", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one search result")
	}

	if results[0].Project != "myproj" {
		t.Errorf("expected Project 'myproj', got %q", results[0].Project)
	}
}
```

- [x] Run test: `go test -run TestIntegration_SyncWithCrosshostEquivalence ./cmd/backscroll/` — should FAIL (will show project as "unknown" before normalization is active)

- [x] After Task 2 is complete (Identify is updated), run test again: should PASS

- [x] Run all cmd/backscroll tests: `go test ./cmd/backscroll/` — should PASS; verify coverage ≥85%

- [x] Commit:
```bash
git add cmd/backscroll/main_test.go
git commit -m "test(integration): verify cross-host cwd resolution in sync pipeline"
```

---

## Task 4: Verify full test suite, coverage, and integration

**Files**
- Test: all modified packages (internal/projects, cmd/backscroll)

**Interfaces**
- Consumes: test suite, database
- Produces: passing tests, ≥85% coverage, working cross-host resolution

**Steps**

- [x] Run full test suite: `just test` — should PASS all tests (including Task 1, 2, 3 new tests)

- [x] Check coverage: `go test -cover ./...` — verify output shows ≥85% for internal/projects and cmd/backscroll

- [x] Run coverage check tool: `just coverage-check` — should PASS (no regressions)

- [x] Run formatter and linter: `just check` — should PASS (gofmt + go vet)

- [x] Smoke test (integration verification): Build backscroll, create a test projects.toml with one Linux root, create a session with macOS cwd, run sync, and verify search returns project ID (not "unknown")
  ```bash
  just build
  # Manual or scripted verification that a cross-host session indexes correctly
  ```

- [x] Commit if any formatting fixes were needed:
```bash
git add -A
git commit -m "chore: formatting and test cleanup"
```

---

## Task 5: Verify readers extract cwd (spot check)

**Files**
- Review (no changes): `internal/readers/claude_reader.go`, `internal/readers/pi_reader.go`, `internal/readers/opencode_reader.go`

**Interfaces**
- Consumes: session JSONL records
- Produces: ParsedFile with Cwd populated (already done; verify implementation)

**Steps**

- [x] Spot-check ClaudeReader.Parse (line 62-70 of claude_reader.go): confirm cwd extraction loop extracts first non-empty cwd from records
- [x] Spot-check PiReader.Parse: confirm cwd extraction pattern (same as ClaudeReader)
- [x] Spot-check OpenCodeReader.Parse: confirm cwd extraction pattern
- [x] Verify all three return ParsedFile with Cwd field populated
- [x] These are already implemented; no code changes needed

---

## Definition of Done

- [x] Tasks 1–5 all completed with checkbox steps marked done
- [x] `just check` passes (gofmt + go vet)
- [x] `just test` passes (all tests, including new integration and cross-host equivalence tests)
- [x] Coverage ≥85% for internal/projects and cmd/backscroll
- [x] All commits follow conventional commit format
- [x] Changes directly to main (no PR, per M1 delivery model)
- [x] Code compiles and is runnable: `just build`

---

## Spec Ambiguities Resolved

1. **Cross-host equivalence semantics**: The spec says "resolution maps equivalent roots through the project registry, not raw string matching." I interpreted this as: a new function `NormalizeRootEquivalence()` checks if cwd and any registry root share the same trailing path components (indicating they are the same logical project from different hosts), then remaps cwd to use the canonical root. This is applied in `Identify()` before all matching logic.

2. **When to normalize**: I chose to normalize early in `Identify()` (before the local hint check) so all subsequent matching uses the canonical form. An alternative would be to normalize only in the subpath-matching step, but early normalization is simpler and more robust.

3. **Trailing component matching threshold**: I required at least 3 path components (`/host/project/subdir`) to reduce false positives. Paths shorter than 3 components are considered too vague to safely map across hosts.

4. **No existing test impact**: The plan adds new tests only; it does not delete or modify existing test expectations (other than checking that cross-host resolution now works).
