# Searchable Tool Calls — Slice 4 (Retire Declarative Engine) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the now-unused declarative input-manifest engine and `JsonlReader`, completing the reader-per-agent migration. Pure deletion + small repoints; no behavior change for the three live readers.

**Architecture:** After Slices 1–3, every input dispatches to a dedicated Go reader (`claude`→ClaudeReader, `pi`→PiReader, `opencode`→OpenCodeReader, `markdown_sections`→sources). `JsonlReader` and the declarative engine (`ParseDeclarative`, `selector`, `predicate`, `transform`, `pipeline`) are dead. This slice deletes them and trims the manifest structs they required.

**Tech Stack:** Go (stdlib), `go-toml/v2` (non-strict — unknown TOML keys are ignored). Tests: stdlib `testing`.

## Global Constraints

- Per-package statement coverage floor ≥85% (pkcov, enforced pre-push and CI via `just coverage-check`). `internal/input_config` and `internal/readers` must stay ≥85% AFTER deletions.
- `gofmt` clean and `go vet` clean (`just check`).
- Pure Go, no CGO.
- Conventional Commits (`type(scope): description`); no AI attribution / Co-Authored-By lines.
- No behavior change for ClaudeReader / PiReader / OpenCodeReader. The full suite stays green at every task boundary (each task leaves the build compiling and tests passing).

## Scope / context an engineer needs (verified blast radius)

- The ONLY production caller of the declarative engine is `internal/readers/jsonl_reader.go` (`JsonlReader.Parse` → `input_config.ParseDeclarativeWithCwd`). `JsonlReader` is registered in `cmd/backscroll/sync_helpers.go` but NO input uses `format="jsonl"` anymore (claude→claude, pi→pi, opencode→opencode, decisions→markdown_sections; `SessionDirsToManifest` synthesizes `format="claude"`).
- Declarative engine files (all in `internal/input_config/`): `selector.go` (`SelectField`/`SelectString`), `predicate.go` (`EvalPredicate(s)`), `transform.go` (`ApplyTransforms`), `pipeline.go` (`ParseDeclarative`, `ParseDeclarativeWithCwd`, `extractRawContent`, `TestFile`, `TestRecord`). None of these are called outside `input_config` once `JsonlReader` is gone. `TestFile`/`TestRecord` have NO production caller.
- Manifest structs in `internal/input_config/types.go` used ONLY by the declarative engine + `SessionDirsToManifest`: `RecordConfig`, `MapConfig`, `ContentConfig`, `TextConfig`, `Predicate`, `RemoveConfig`, and the `Record`/`Map`/`Content`/`Text` fields of `InputDefinition`. The readers ignore the `InputDefinition` body (all three `Parse` methods take `_ input_config.InputDefinition`); they use only `def.Discover` (via `Discover`) and `def.Decode.Format` (via dispatch).
- `compat.go` `SessionDirsToManifest` currently POPULATES `Record/Map/Content/Text` — must be simplified to set only `ID/Source/Active/Discover/Decode`. `ActiveInputs`, `InputMode`, `ModeDeclarative` stay (used by `status`/`config`).
- `go-toml/v2` is non-strict here (`toml.Unmarshal(data, &f)` in `loader.go`), so user manifests that still contain `[inputs.record]`/`[inputs.map]`/`[inputs.content]`/`[inputs.text]` load fine after the struct fields are removed — the extra keys are ignored. The shipped `opencode.inputs.toml` and `decisions.inputs.toml` have no declarative blocks.
- Tests referencing the deleted symbols: `internal/readers/jsonl_reader_test.go`, `internal/readers/integration_test.go`, `internal/readers/reader_test.go` (ForDef empty-format fallback), `internal/input_config/{selector,predicate,pipeline,transform}_test.go`, `internal/input_config/{compat,types}_test.go`, and `cmd/backscroll/main_test.go` (`setupInputsPreset` writes `format="jsonl"`).
- `internal/reader` (the `read` command) uses `sync.ParseSessions` — a SEPARATE Go parser, NOT the declarative engine. Do NOT touch it.
- Design source of truth: `docs/superpowers/specs/2026-06-29-searchable-tool-calls-reader-per-agent-design.md`.

## File Structure

- Delete: `internal/readers/jsonl_reader.go`, `internal/readers/jsonl_reader_test.go`.
- Delete: `internal/input_config/selector.go` (+`_test`), `predicate.go` (+`_test`), `transform.go` (+`_test`), `pipeline.go` (+`_test`).
- Modify: `cmd/backscroll/sync_helpers.go` (drop JsonlReader registration).
- Modify: `internal/readers/reader.go` (ForDef/Default fallback `jsonl`→`claude`).
- Modify: `internal/readers/reader_test.go`, `internal/readers/integration_test.go`, `cmd/backscroll/main_test.go` (drop jsonl/declarative usage).
- Modify: `internal/input_config/types.go` (trim structs), `compat.go` (simplify `SessionDirsToManifest`), `compat_test.go`, `types_test.go`.
- Modify: `CLAUDE.md` (descriptions).

---

### Task 1: Delete JsonlReader and repoint dispatch

**Files:**
- Delete: `internal/readers/jsonl_reader.go`, `internal/readers/jsonl_reader_test.go`
- Modify: `cmd/backscroll/sync_helpers.go` (remove `reg.Register(&readers.JsonlReader{})` at line 35)
- Modify: `internal/readers/reader.go` (fallback format `"jsonl"`→`"claude"` in `ForDef`; update `Default` + comments)
- Modify: `internal/readers/reader_test.go` (empty-format fallback expectation)
- Modify: `internal/readers/integration_test.go` (swap readers; delete declarative-only tests)
- Modify: `cmd/backscroll/main_test.go` (`setupInputsPreset` format)

**Interfaces:**
- Consumes: existing `ClaudeReader`, `PiReader`, `OpenCodeReader`.
- Produces: no new symbols; removes `JsonlReader`. `Registry.ForDef` empty-format default becomes `"claude"`.

- [ ] **Step 1: Delete JsonlReader files**

```bash
git rm internal/readers/jsonl_reader.go internal/readers/jsonl_reader_test.go
```

- [ ] **Step 2: Remove the registration**

In `cmd/backscroll/sync_helpers.go`, delete the line `reg.Register(&readers.JsonlReader{})` (around line 35). Leave the `ClaudeReader`, `PiReader`, `OpenCodeReader`, and sources registrations intact.

- [ ] **Step 3: Repoint the dispatch fallback**

In `internal/readers/reader.go`:
- In `ForDef`, change the empty-format fallback from `format = "jsonl"` to `format = "claude"`, and update the doc comment from `// Falls back to "jsonl" if the format is empty.` to `// Falls back to "claude" if the format is empty.`
- In `Default`, change the preferred key lookup from `r.readers["jsonl"]` to `r.readers["claude"]` and update its comment to `// Default returns the "claude" reader, or the first registered reader if claude is absent.`
- Update the `Name` interface comment example if it says `"jsonl"` → use `"claude"`.

- [ ] **Step 4: Fix reader_test.go**

In `internal/readers/reader_test.go`, the `ForDef` test exercises the empty-format fallback expecting `"jsonl"`. Update that case to register a mock named `"claude"` and assert the empty-format `ForDef` returns `"claude"`. The `Default` test (`TestDefault`) and the duplicate-registration/`Get` tests use mock readers named `"jsonl"` as arbitrary labels — if `TestDefault` asserts `Default()` prefers `"jsonl"`, update it to register a mock `"claude"` and expect `"claude"` (matching the new `Default` logic). Keep all other mock-registry tests as-is.

- [ ] **Step 5: Rewrite integration_test.go**

In `internal/readers/integration_test.go`:
- `TestPipeline_ClaudeJSONL`: change `r := &JsonlReader{}` to `r := &ClaudeReader{}` and replace the `input_config.InputDefinition{...}` literal passed to `Parse` with `input_config.InputDefinition{}` (the reader ignores it). Keep the Claude fixture and the assertions; if an assertion depends on the old declarative message-merging (e.g. exact record count), relax it to assert that the fixture's text content appears in `pf.Records` (ClaudeReader may split text and tool blocks into separate messages). Run the test and adjust assertions to the observable behavior.
- `TestPipeline_PiJSONL`: same, with `r := &PiReader{}`.
- `TestPipeline_OpenCode`: unchanged (already `OpenCodeReader`); keep its helper `createTestOpenCodeDB`.
- Delete `TestPipeline_Dedup`, `TestPipeline_Predicates`, and `TestPipeline_TextTransforms` entirely — `Predicates`/`TextTransforms` test declarative features that no longer exist; `Dedup` tested `JsonlReader` hash stability already covered by hashfile.

- [ ] **Step 6: Fix main_test.go preset helper**

In `cmd/backscroll/main_test.go`, `setupInputsPreset` writes a manifest with `format = "jsonl"`. Change that to `format = "claude"`. (The declarative `[inputs.*]` blocks in that TOML string may stay — they are ignored — but the format must route to a registered reader.)

- [ ] **Step 7: Build + test + vet + fmt**

Run: `gofmt -w . && go vet ./... && go build ./... && go test ./internal/readers/ ./cmd/backscroll/`
Expected: PASS. `pipeline.go`/`selector.go`/etc. still exist (unused now) and compile.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "refactor(readers): remove JsonlReader; default dispatch to claude"
```

---

### Task 2: Delete the declarative engine

**Files:**
- Delete: `internal/input_config/selector.go` (+ `selector_test.go`), `predicate.go` (+ `predicate_test.go`), `transform.go` (+ `transform_test.go`), `pipeline.go` (+ `pipeline_test.go`)

**Interfaces:**
- Consumes: nothing (these are now unreferenced after Task 1).
- Produces: removes `SelectField`, `SelectString`, `EvalPredicate(s)`, `ApplyTransforms`, `ParseDeclarative(WithCwd)`, `extractRawContent`, `TestFile`, `TestRecord`, `ErrDropped`, `InvalidPatternError`.

- [ ] **Step 1: Confirm no remaining references**

Run: `rg -n "SelectField|SelectString|EvalPredicate|ApplyTransforms|ParseDeclarative|extractRawContent|\bTestFile\b|TestRecord" --type go`
Expected: only matches inside the four files about to be deleted (and their tests). If anything else references them, STOP and report — Task 1 missed a caller.

- [ ] **Step 2: Delete the engine files**

```bash
git rm internal/input_config/selector.go internal/input_config/selector_test.go \
       internal/input_config/predicate.go internal/input_config/predicate_test.go \
       internal/input_config/transform.go internal/input_config/transform_test.go \
       internal/input_config/pipeline.go internal/input_config/pipeline_test.go
```

- [ ] **Step 3: Build + vet**

Run: `go build ./... && go vet ./...`
Expected: PASS. `types.go` still defines `RecordConfig`/`MapConfig`/`ContentConfig`/`TextConfig`/`Predicate`/`RemoveConfig` (referenced by `compat.go` and `types_test.go` until Task 3), so the package compiles.

- [ ] **Step 4: Run the input_config + full suite**

Run: `go test ./internal/input_config/ ./...`
Expected: PASS (the deleted files' tests are gone; remaining tests green).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(input_config): delete declarative pipeline, selector, predicate, transform"
```

---

### Task 3: Trim manifest structs and simplify SessionDirsToManifest

**Files:**
- Modify: `internal/input_config/types.go` (remove dead structs + `InputDefinition` fields)
- Modify: `internal/input_config/compat.go` (`SessionDirsToManifest`)
- Modify: `internal/input_config/compat_test.go`, `internal/input_config/types_test.go`

**Interfaces:**
- Consumes: trimmed `InputDefinition` (now `ID`, `Source`, `Active`, `Discover`, `Decode`).
- Produces: removes `RecordConfig`, `MapConfig`, `ContentConfig`, `TextConfig`, `Predicate`, `RemoveConfig`.

- [ ] **Step 1: Trim types.go**

In `internal/input_config/types.go`:
- Remove the `Record`, `Map`, `Content`, `Text` fields from `InputDefinition` (keep `ID`, `Source`, `Active`, `Discover`, `Decode`, and any version/metadata fields).
- Delete the struct definitions `RecordConfig`, `MapConfig`, `ContentConfig`, `TextConfig`, `Predicate`, `RemoveConfig`.
- Keep `InputFile`, `InputDefinition` (trimmed), `DiscoverConfig`, `DecodeConfig`.

- [ ] **Step 2: Simplify SessionDirsToManifest**

In `internal/input_config/compat.go`, replace the `SessionDirsToManifest` body so it returns only the discovery + decode manifest:

```go
func SessionDirsToManifest(dirs []string) InputDefinition {
	return InputDefinition{
		ID:     "legacy-session-dirs",
		Source: "session",
		Active: true,
		Discover: DiscoverConfig{
			Roots:          dirs,
			Include:        []string{"**/*.jsonl"},
			Exclude:        []string{"**/subagents/**"},
			FollowSymlinks: false,
		},
		Decode: DecodeConfig{Format: "claude"},
	}
}
```

Update its doc comment to drop any mention of declarative selectors/predicates.

- [ ] **Step 3: Fix compat_test.go and types_test.go**

Remove assertions that reference the deleted fields/structs (`Record.IncludeWhen`, `Map.Role`, `Content.*`, `Text.*`, `Predicate`, etc.). Keep assertions on `ID`, `Source`, `Active`, `Discover` (roots/include/exclude), and `Decode.Format == "claude"`. For `types_test.go`, drop any test that constructed or asserted the removed structs.

- [ ] **Step 4: Build + test + coverage**

Run: `gofmt -w . && go vet ./... && go build ./... && just test`
Expected: PASS.

Run: `just coverage-check`
Expected: all packages ≥85%, including `internal/input_config` and `internal/readers`. If `internal/input_config` dips below 85% (the deletions removed heavily-tested code, leaving loader/discover/compat), add focused tests for any now-uncovered branch in `loader.go`/`discover.go`/`compat.go` until the floor is met, and report what was added. If it cannot reach 85% without contrived tests, STOP and report — the floor for that package may need review.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor(input_config): trim manifest structs to discovery + decode"
```

---

### Task 4: Docs

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:** none (documentation only).

- [ ] **Step 1: Update CLAUDE.md**

- In the `internal/` Module Layout tree, the `input_config/` line currently reads `— declarative input manifest engine: types, loader, discovery, predicates, transforms`. Change it to reflect the trimmed package, e.g. `— input manifest loading, discovery, and legacy session-dirs compatibility`.
- In the `readers/` Module Layout line and the Package Layout `internal/readers` row, remove `JsonlReader` from the list (keep `SessionReader interface, Registry, ClaudeReader (...), PiReader (...), OpenCodeReader (...); toolfmt serializer`).
- In the Package Layout table, update the `internal/input_config` row description to match (drop "declarative input manifest engine (*.inputs.toml)" wording referencing predicates/transforms; it is now manifest loading/discovery/compat).
- Scan Key Design Decisions and the intro for any remaining mention of a "declarative pipeline"/"declarative engine" driving session parsing and correct it (parsing is now per-agent Go readers).

- [ ] **Step 2: Verify**

Run: `just check`
Expected: PASS. No package was added or deleted (all `internal/*` packages still exist), so the pre-push Module/Package Layout gate is satisfied as long as the text is accurate.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: retire declarative engine references; reader-per-agent is the parser layer"
```

---

## Self-Review

**Spec coverage (Slice 4 of the design — retire declarative engine):**
- Delete `selector`/`predicate`/`transform`/`ParseDeclarative` → Task 2. ✓
- Delete `JsonlReader` → Task 1. ✓
- Trim `types.go` (drop `RecordConfig`/`MapConfig`/`ContentConfig`/`TextConfig`/`Predicate`/`RemoveConfig`) → Task 3. ✓
- Keep `loader.go`, `discover.go`, `compat.go`, trimmed `types.go` (still used by `status`/`config` and all readers for discovery) → Tasks 2–3 leave them intact. ✓
- No behavior change for the three readers; dispatch fallback repointed to `claude` → Task 1. ✓
- Docs corrected → Task 4. ✓

**Build-green ordering:** Task 1 removes the only declarative caller (JsonlReader), leaving the engine files compiling-but-unused. Task 2 deletes the now-unreferenced engine files (structs in `types.go` still satisfy `compat.go`/tests). Task 3 removes those structs and simplifies the one populator (`SessionDirsToManifest`) together. Each task ends with a green build + suite.

**Placeholder scan:** Deletions use exact `git rm` paths; edits name exact files, symbols, and before/after text. The integration-test assertion relaxation is an explicit, bounded instruction (assert content presence; adjust by running the test), not a vague directive. ✓

**Type consistency:** After Task 3, `InputDefinition` = `{ID, Source, Active, Discover, Decode}` (+ version/metadata). `DiscoverConfig`/`DecodeConfig` unchanged. All three readers already take `_ input_config.InputDefinition` and use only `Discover`/`Decode` — unaffected by the field removals. `ForDef`/`Default` consistently key on `"claude"`. ✓

## After Slice 4

The reader-per-agent migration is complete: Claude, Pi, and OpenCode each have a dedicated Go reader indexing text + tool inputs + tool outputs; the declarative TOML engine is gone; manifests carry only discovery + decode-format. No further slices.
