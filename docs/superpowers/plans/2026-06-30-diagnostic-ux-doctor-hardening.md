# Diagnostic UX & Doctor Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add actionable stderr guidance to `search`/`list` (zero-result hints and a short tool-query warning) and cut input noise from the `backscroll-doctor` diagnostic script.

**Architecture:** Two pure helper functions in a new `cmd/backscroll/diagnostics.go`, each writing only to the caller-supplied stderr writer so STDOUT (and `--json`) stays untouched. Wire them into `runSearch` and `runList`. Separately, widen the `gather.sh` noise regex.

**Tech Stack:** Go (stdlib `io`/`fmt`/`strings`), cobra CLI, stdlib `testing` with the existing `run(stdout, stderr, args)` harness; Bash for the skill asset.

## Global Constraints

- Formatting: `gofmt` clean; `go vet` clean (`just check`).
- Tests: `just test` green; per-package statement coverage ≥85% (pkcov, enforced pre-push and in CI).
- Hints/warnings go to STDERR only. STDOUT (text, robot, and `--json`) must be byte-identical to today.
- No new package, no new dependency, no schema migration.
- Any non-test change under `cmd/` requires a docs touch in the same push range (pre-push gate) — Task 1 updates CLAUDE.md to cover it.
- Commits: Conventional Commits; end message with `Claude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy`.
- All code artifacts (identifiers, comments, strings) in English.

---

### Task 1: Zero-result hints for search and list

**Files:**
- Create: `cmd/backscroll/diagnostics.go`
- Create: `cmd/backscroll/diagnostics_test.go`
- Modify: `cmd/backscroll/search.go` (inside `runSearch`, after `results` is computed, ~line 172)
- Modify: `cmd/backscroll/list.go` (inside `runList`, the `if len(sessions) == 0` branch, ~line 119)
- Modify: `CLAUDE.md` (search/list command description — one line about zero-result hints)

**Interfaces:**
- Produces: `func writeSearchHints(w io.Writer, allProjects, alreadyToolScoped bool)` — writes a "no results" suggestion block to `w`. Consumed by Task 2's wiring is independent (Task 2 adds a different function).

- [ ] **Step 1: Write the failing test**

Create `cmd/backscroll/diagnostics_test.go`:

```go
package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteSearchHints_ProjectScoped(t *testing.T) {
	var buf bytes.Buffer
	writeSearchHints(&buf, false, false)
	out := buf.String()
	if !strings.Contains(out, "--all-projects") {
		t.Errorf("project-scoped hint should suggest --all-projects, got:\n%s", out)
	}
	if !strings.Contains(out, "--content-type tool") {
		t.Errorf("non-tool query should suggest --content-type tool, got:\n%s", out)
	}
	if !strings.Contains(out, "backscroll status") {
		t.Errorf("hint should mention backscroll status, got:\n%s", out)
	}
}

func TestWriteSearchHints_AllProjectsAndToolScoped(t *testing.T) {
	var buf bytes.Buffer
	writeSearchHints(&buf, true, true)
	out := buf.String()
	if strings.Contains(out, "--all-projects") {
		t.Errorf("already all-projects: should not suggest --all-projects, got:\n%s", out)
	}
	if strings.Contains(out, "--content-type tool") {
		t.Errorf("already tool-scoped: should not suggest --content-type tool, got:\n%s", out)
	}
	if !strings.Contains(out, "backscroll status") {
		t.Errorf("status hint should always appear, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestWriteSearchHints ./cmd/backscroll/`
Expected: FAIL — `undefined: writeSearchHints`.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/backscroll/diagnostics.go`:

```go
package main

import (
	"fmt"
	"io"
)

// writeSearchHints prints actionable suggestions to w after a query returned zero
// rows. It writes only to w (stderr) so STDOUT — including --json — stays a clean,
// parseable empty payload. allProjects suppresses the --all-projects suggestion;
// alreadyToolScoped suppresses the --content-type tool suggestion.
func writeSearchHints(w io.Writer, allProjects, alreadyToolScoped bool) {
	fmt.Fprintln(w, "no results — suggestions:")
	if !allProjects {
		fmt.Fprintln(w, "  • --all-projects: search across every project, not just the current one")
	}
	if !alreadyToolScoped {
		fmt.Fprintln(w, "  • --content-type tool: match commands, file paths, and errors")
	}
	fmt.Fprintln(w, "  • backscroll status: confirm the index is up to date")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestWriteSearchHints ./cmd/backscroll/`
Expected: PASS.

- [ ] **Step 5: Wire into runSearch**

In `cmd/backscroll/search.go`, immediately after the `results, err := db.HybridSearch(query, opts)` error check (before the `modelResults` conversion, ~line 173), add:

```go
	if len(results) == 0 {
		writeSearchHints(stderr, allProjects, contentType == "tool")
	}
```

- [ ] **Step 6: Wire into runList**

In `cmd/backscroll/list.go`, inside the post-query `if len(sessions) == 0 {` branch (~line 119), after the existing STDOUT empty-message lines and before `return nil`, add:

```go
		writeSearchHints(stderr, allProjects, true)
```

(`alreadyToolScoped=true` because `list` has no content-type filter, so the tool suggestion does not apply.)

- [ ] **Step 7: Write the integration test**

Append to `cmd/backscroll/diagnostics_test.go`:

```go
func TestSearchZeroResultHintsToStderr(t *testing.T) {
	// A query that cannot match anything; --json keeps stdout a clean empty array.
	out, stderr, err := runCmd("search", "zzqqxx_no_such_token_zzqqxx", "--json", "--indexed-only")
	if err != nil {
		t.Fatalf("search error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "no results") {
		t.Errorf("expected zero-result hint on stderr, got: %q", stderr)
	}
	if strings.Contains(out, "•") || strings.Contains(out, "suggestions") {
		t.Errorf("hints must not leak into stdout, got: %q", out)
	}
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./cmd/backscroll/`
Expected: PASS. If `runCmd` is defined in `main_test.go` (it is), the integration test compiles against it.

- [ ] **Step 9: Update CLAUDE.md (satisfies docs gate)**

In `CLAUDE.md`, in the "Key Design Decisions" list, add one bullet:

```markdown
- **Zero-result guidance**: when `search`/`list` return no rows, actionable suggestions (`--all-projects`, `--content-type tool`, `backscroll status`) are printed to STDERR — never STDOUT, so `--json` stays a clean empty payload.
```

- [ ] **Step 10: Verify formatting, vet, coverage**

Run: `just check && go test -cover ./cmd/backscroll/`
Expected: no gofmt/vet output; coverage ≥85%.

- [ ] **Step 11: Commit**

```bash
git add cmd/backscroll/diagnostics.go cmd/backscroll/diagnostics_test.go cmd/backscroll/search.go cmd/backscroll/list.go CLAUDE.md
git commit -m "$(printf 'feat(cli): print actionable hints on zero-result search/list\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 2: Short tool-query warning (trigram floor)

**Files:**
- Modify: `cmd/backscroll/diagnostics.go` (add `warnShortToolQuery`)
- Modify: `cmd/backscroll/diagnostics_test.go` (add tests)
- Modify: `cmd/backscroll/search.go` (call early in `runSearch`)

**Interfaces:**
- Consumes: nothing from Task 1 (same file, independent function).
- Produces: `func warnShortToolQuery(w io.Writer, contentType, query string)` — self-guarding; warns only when `contentType == "tool"` and the trimmed query is under 3 runes.

- [ ] **Step 1: Write the failing test**

Append to `cmd/backscroll/diagnostics_test.go`:

```go
func TestWarnShortToolQuery(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		query       string
		wantWarn    bool
	}{
		{"short tool query warns", "tool", "go", true},
		{"short tool query with spaces warns", "tool", " cd ", true},
		{"three-char tool query is fine", "tool", "git", false},
		{"short query but not tool-scoped", "", "go", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			warnShortToolQuery(&buf, tc.contentType, tc.query)
			got := strings.Contains(buf.String(), "under 3 characters")
			if got != tc.wantWarn {
				t.Errorf("warn=%v want=%v (out=%q)", got, tc.wantWarn, buf.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestWarnShortToolQuery ./cmd/backscroll/`
Expected: FAIL — `undefined: warnShortToolQuery`.

- [ ] **Step 3: Write minimal implementation**

Append to `cmd/backscroll/diagnostics.go` (add `"strings"` to the import block):

```go
// warnShortToolQuery warns when a tool-scoped query is too short for the tool_fts
// trigram tokenizer, which needs ≥3 characters and will otherwise match nothing.
// Self-guarding: does nothing unless contentType == "tool" and the trimmed query
// is under 3 runes. The query still runs; this is advisory only.
func warnShortToolQuery(w io.Writer, contentType, query string) {
	if contentType != "tool" {
		return
	}
	if len([]rune(strings.TrimSpace(query))) < 3 {
		fmt.Fprintf(w, "warning: %q is under 3 characters; the tool index (trigram) needs ≥3 and will match nothing\n", strings.TrimSpace(query))
	}
}
```

The import block becomes:

```go
import (
	"fmt"
	"io"
	"strings"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestWarnShortToolQuery ./cmd/backscroll/`
Expected: PASS.

- [ ] **Step 5: Wire into runSearch**

In `cmd/backscroll/search.go`, right after the `--fields` validation block (before `config.Load()`, ~line 111), add:

```go
	warnShortToolQuery(stderr, contentType, query)
```

- [ ] **Step 6: Write the integration test**

Append to `cmd/backscroll/diagnostics_test.go`:

```go
func TestSearchShortToolQueryWarnsToStderr(t *testing.T) {
	out, stderr, err := runCmd("search", "go", "--content-type", "tool", "--json", "--indexed-only")
	if err != nil {
		t.Fatalf("search error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "under 3 characters") {
		t.Errorf("expected short-query warning on stderr, got: %q", stderr)
	}
	if strings.Contains(out, "under 3 characters") {
		t.Errorf("warning must not leak into stdout, got: %q", out)
	}
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./cmd/backscroll/`
Expected: PASS.

- [ ] **Step 8: Verify formatting, vet, coverage**

Run: `just check && go test -cover ./cmd/backscroll/`
Expected: clean; coverage ≥85%.

- [ ] **Step 9: Commit**

```bash
git add cmd/backscroll/diagnostics.go cmd/backscroll/diagnostics_test.go cmd/backscroll/search.go
git commit -m "$(printf 'feat(cli): warn when a tool query is under the trigram 3-char floor\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 3: gather.sh noise hardening

**Files:**
- Modify: `.claude/skills/backscroll-doctor/assets/gather.sh` (widen `NOISE`)
- Modify: `.claude/skills/backscroll-doctor/SKILL.md` (reaffirm tool-error hits are leads)

**Interfaces:** none (shell asset + doc; no Go).

- [ ] **Step 1: Widen the NOISE regex**

In `.claude/skills/backscroll-doctor/assets/gather.sh`, replace the `NOISE` assignment:

```bash
NOISE='encrypted_content|pi-drive:observation|system-reminder|task-notification'
```

with:

```bash
# Strip Pi reasoning/telemetry blobs, harness chatter, and the doctor's own
# self-referential output (its script + skill name) — all pure noise.
NOISE='encrypted_content|pi-drive:observation|turn_end|turn_start|turn-end|turn-start|system-reminder|task-notification|gather\.sh|backscroll-doctor'
```

- [ ] **Step 2: Verify the turn-telemetry false positives are gone**

Run:
```bash
cd /Users/Shared/harness/backscroll
BACKSCROLL_DOCTOR_LIMIT=8 .claude/skills/backscroll-doctor/assets/gather.sh errors | rg -c "turn_end|turn_start" || echo "0 turn-telemetry lines (expected)"
```
Expected: prints `0 turn-telemetry lines (expected)` (rg finds no matches, exits non-zero, `||` branch runs).

- [ ] **Step 3: Confirm the script still produces real signal**

Run:
```bash
BACKSCROLL_DOCTOR_LIMIT=8 .claude/skills/backscroll-doctor/assets/gather.sh errors | rg -c "database is locked|SQLITE_BUSY" || echo "no lock signatures found"
```
Expected: a non-zero count (the historical, already-fixed lock signatures) OR the fallback line — either proves the pipe still works after the regex change.

- [ ] **Step 4: Reaffirm the verify step in SKILL.md**

In `.claude/skills/backscroll-doctor/SKILL.md`, under the "Errors/bugs" bullet in Execution Steps, append a sentence so the framing is explicit:

Change:
```markdown
   - **Errors/bugs** — failed tool outputs: `assets/gather.sh errors`.
```
to:
```markdown
   - **Errors/bugs** — failed tool outputs: `assets/gather.sh errors`. Trigram matching on tool content yields prose false positives; treat every error-signature hit as a LEAD, never a fact — the verify step (4) is the guard.
```

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/backscroll-doctor/assets/gather.sh .claude/skills/backscroll-doctor/SKILL.md
git commit -m "$(printf 'fix(skills): harden backscroll-doctor gather.sh noise filter\n\nDrop Pi turn telemetry and self-referential (gather.sh / backscroll-doctor)\nlines that produced false-positive error signatures, and reaffirm that\ntool-error hits are unverified leads.\n\nClaude-Session: https://claude.ai/code/session_01Wty6pS4yEw8sRgrac1BtGy')"
```

---

### Task 4: Final gate + push

- [ ] **Step 1: Full local gates**

Run: `just check && just test`
Expected: gofmt/vet clean; all packages `ok`.

- [ ] **Step 2: Coverage floor**

Run: `bash scripts/check-coverage.sh` (or `just coverage-check`)
Expected: passes; `cmd/backscroll` ≥85%.

- [ ] **Step 3: Push (runs pre-push gates)**

Run: `git push`
Expected: coverage check passes, skills reinstalled, binary rebuilt, refs updated.

---

## Self-Review

**Spec coverage:**
- C1 zero-result diagnostics → Task 1 (helper + wiring in both commands + integration test). ✓
- C2 short tool-query warning → Task 2 (helper + wiring + integration test). ✓
- C3 gather.sh noise hardening → Task 3 (NOISE regex + SKILL.md note). ✓
- Testing requirements (stderr captured, stdout unchanged, ≥85%) → Tasks 1/2 tests + Task 4 gates. ✓
- Docs-update gate → Task 1 Step 9 (CLAUDE.md). ✓

**Placeholder scan:** none — every code/step block is complete.

**Type consistency:** `writeSearchHints(io.Writer, bool, bool)` and `warnShortToolQuery(io.Writer, string, string)` are used with matching signatures in wiring and tests. `runCmd` matches the existing `main_test.go` helper `(args ...string) (stdout, stderr string, err error)`.

**Note for implementer:** the design mentioned making the tool suggestion conditional on the query "looking like" a path/command. That heuristic was intentionally dropped for a deterministic rule — always suggest `--content-type tool` when not already tool-scoped. This is simpler and testable; do not add a heuristic.
