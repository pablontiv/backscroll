# Manifest Source Ingestion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `*.inputs.toml` manifests the only external-source truth, preflight every active input before sync, and retrieve nested/new decision Markdown through a registered `markdown_sections` reader.

**Architecture:** Extend the stateless `internal/compat` boundary with manifest types only when this plan consumes them, while keeping file discovery, decoding, and `SyncFiles` execution in their existing consumers. Register one normal `SessionReader` for Markdown sections and reuse `input_config.DiscoverFiles` plus `readers.Registry`. Plan 3’s already-active fail-closed policy guarantees that any manifest failure blocks cached index output.

**Tech Stack:** Go 1.26.2, `pelletier/go-toml/v2`, existing `internal/input_config`, existing `internal/readers`, SHA-256 hashing, Cobra CLI integration tests, Just

**Spec:** `docs/superpowers/specs/2026-08-18-systemic-index-compatibility-design.md`

## Global Constraints

- This is delivery Plan 2 but executes third: Plan 1 inspection/migration primitives → Plan 3 recovery and blocking-policy activation → Plan 2 manifest ingestion → Plan 4 shipped guidance validation.
- This plan depends on Plan 1’s core `compat.Diagnostic` and Plan 3’s active command policy, stale-index behavior, direct-read exemption, and executable `recover` continuation. It introduces `ManifestPlan`, `ResolvedInput`, and `ReaderLookup` only in Task 2 when preflight first consumes them.
- `*.inputs.toml` is the sole external-source truth. Non-empty legacy `[sources]` is rejected; no config mutation, manifest generation, warning-only path, or dual ingestion is allowed.
- Legacy rejection prints a complete exact manifest using the observed category and paths.
- Every active manifest is loaded and registry-resolved before discovery, hashing, parsing, or syncing any input.
- Invalid TOML, unsupported manifest version, duplicate input ID, missing required field, unknown decoder, invalid/inaccessible root, and unsafe pattern are visible blocking failures in deterministic manifest-file/input-ID order.
- An existing valid root with no matching files is a valid empty input. A missing or inaccessible declared root is invalid.
- `markdown_sections` is a normal `readers.SessionReader`; it uses existing manifest discovery and deterministic hashing, emits ordered eligible sections, and falls back to one whole-document record when no eligible heading exists.
- Parse all planned files before calling existing `storage.SyncFiles`; any failure propagates through Plan 3’s active stale policy and no indexed command serves cached rows.
- Direct `read` remains available and makes no index-freshness claim.
- Tests set `HOME` and `BACKSCROLL_CONFIG_DIR` to `t.TempDir()` and use no network, ambient tags, or user files.
- Follow strict RED → GREEN → TRIANGULATE → REFACTOR. Run focused tests before `just check`, `just test`, and `just ci`.
- Commit commands below are future execution instructions only. Do not stage or commit while writing or reviewing this plan.

---

## File map

| Path | Responsibility |
|---|---|
| `internal/compat/manifest.go` | Inspect raw `[sources]` and active definitions; return typed diagnostics or resolved input plans. |
| `internal/compat/manifest_test.go` | Exact conversion, precedence, duplicate/version/root/decoder preflight tests. |
| `internal/config/config.go` | Expose deterministic raw loaded config bytes without changing effective config behavior. |
| `internal/input_config/loader.go` | Load all definitions with file provenance before filtering active entries. |
| `internal/input_config/types.go` | Add manifest provenance and validation result types used by preflight. |
| `internal/input_config/discover.go` | Return declared-root errors instead of silently skipping invalid roots. |
| `internal/readers/markdown_sections.go` | Discover, hash, and parse Markdown sections through `SessionReader`. |
| `internal/readers/markdown_sections_test.go` | Ordered sections, heading levels, frontmatter, and whole-document fallback. |
| `internal/readers/reader.go` | Provide one production registry constructor containing all built-in readers. |
| `cmd/backscroll/sync_helpers.go` | Run all-active preflight, then discovery/parse, then one existing sync execution. |
| `cmd/backscroll/manifest_ingestion_test.go` | CLI-level preflight, no-partial-sync, legacy rejection, and decision retrieval. |
| `inputs/decisions.inputs.toml` | Active repaired decision preset using `markdown_sections` and real command instructions. |
| `tests/fixtures/decisions/**` | Nested/new Markdown fixtures for end-to-end retrieval. |

### Task 1: Load manifest provenance and reject legacy `[sources]` exactly

**Files:**
- Modify: `internal/compat/types.go`
- Create: `internal/compat/manifest.go`
- Create: `internal/compat/manifest_test.go`
- Modify: `internal/config/config.go:42-115`
- Modify: `internal/input_config/types.go:3-29`
- Modify: `internal/input_config/loader.go:27-68`

**Interfaces:**
- Consumes: core `compat.Diagnostic`, existing `input_config.InputDefinition`, and TOML parser.
- Produces: `const compat.CodeLegacySources Code = "legacy_sources"`, `type input_config.LoadedDefinition struct { File string; Definition InputDefinition }`, `func input_config.LoadAllFromDir(dir string) ([]LoadedDefinition, error)`, `type config.LoadedFile struct { Path string; Data []byte }`, `func config.LoadedConfigFiles() ([]LoadedFile, error)`, and `func compat.InspectLegacyConfig(raw []byte) *Diagnostic`.

- [ ] **Step 1: Write the exact legacy conversion failure test**

```go
func TestLegacySourcesRejectedWithExactManifestExample(t *testing.T) {
    raw := []byte("[sources]\ndecisions = [\"/work/project/docs/decisions\"]\n")
    d := InspectLegacyConfig(raw)
    if d == nil || d.Code != CodeLegacySources { t.Fatalf("diagnostic=%+v", d) }
    for _, want := range []string{
        `id = "decisions"`, `source = "decision"`,
        `roots = ["/work/project/docs/decisions"]`, `include = ["**/*.md"]`,
        `format = "markdown_sections"`,
    } {
        if !strings.Contains(d.Summary, want) { t.Fatalf("summary missing %q: %s", want, d.Summary) }
    }
}
```

At CLI level, snapshot config and database bytes before the call and assert neither changes.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/compat -run '^TestLegacySourcesRejectedWithExactManifestExample$'`

Expected: FAIL with `undefined: InspectLegacyConfig`.

- [ ] **Step 3: Implement raw config inspection and provenance-preserving loads**

Add only `CodeLegacySources` to `internal/compat/types.go` because this task is its first consumer. `LoadedConfigFiles` returns readable global then local config bytes in the same precedence order `config.Load` already applies. `LoadAllFromDir` sorts `*.inputs.toml` paths, requires `version == 1`, retains inactive entries for validation, and records each source file. `InspectLegacyConfig` parses only enough TOML to observe `sources`, orders categories as `backlog`, `decisions`, `ke`, `memories`, `rules`, `specs`, and returns one complete manifest block per non-empty category. Its continuation is `[]string{"config"}` because `backscroll config` is executable and exposes the manifest location; the summary itself carries the repair text.

- [ ] **Step 4: GREEN and triangulate empty/invalid config**

Run: `go test ./internal/compat ./internal/input_config ./internal/config -run '^(TestLegacySourcesRejectedWithExactManifestExample|TestEmptyLegacySourcesAllowed|TestLegacySourcesPreserveCategoryAndPathOrder|TestLoadAllFromDirRejectsUnsupportedVersion|TestLoadedConfigFilesFollowEffectivePrecedence)$'`

Expected: PASS; empty `[sources]` returns nil, invalid TOML remains precedence level 1 as a Go parse error, and no test reads real HOME.

- [ ] **Step 5: Refactor exact manifest formatting and run affected packages**

Run: `go test ./internal/compat ./internal/input_config ./internal/config`

Expected: PASS.

- [ ] **Step 6: Commit legacy rejection and provenance as one review unit**

```bash
git add internal/compat/types.go internal/compat/manifest.go internal/compat/manifest_test.go internal/config/config.go internal/input_config/types.go internal/input_config/loader.go
git commit -m "feat(inputs): reject legacy sources with exact manifests"
```

### Task 2: Preflight every active manifest before any discovery or sync

**Files:**
- Modify: `internal/compat/types.go`
- Modify: `internal/compat/manifest.go`
- Modify: `internal/compat/manifest_test.go`
- Modify: `internal/input_config/discover.go:16-51`
- Modify: `internal/readers/reader.go:25-75`
- Modify: `cmd/backscroll/sync_helpers.go:50-128`
- Create: `cmd/backscroll/manifest_ingestion_test.go`

**Interfaces:**
- Consumes: `input_config.LoadedDefinition`, existing `Registry.ForDef(InputDefinition) (SessionReader, error)`, and core diagnostics.
- Produces: constants `CodeInvalidManifest = "invalid_manifest"`, `CodeUnknownDecoder = "unknown_decoder"`, and `CodeInvalidRoot = "invalid_root"`; `type ReaderLookup interface { ForDef(input_config.InputDefinition) (readers.SessionReader, error) }`, `type ManifestPlan struct { Inputs []ResolvedInput }`, `type ResolvedInput struct { Definition input_config.InputDefinition; ReaderName string }`, `func InspectManifests(defs []input_config.LoadedDefinition, registry ReaderLookup) (ManifestPlan, *Diagnostic, error)`, `func readers.NewDefaultRegistry() *Registry`, and `func preflightInputs(cfg *config.Config) (compat.ManifestPlan, *compat.Diagnostic, error)`.

- [ ] **Step 1: Write all-active preflight tests**

Add `TestActiveManifestsPreflightBeforeSync` as a table with invalid TOML, unsupported version, duplicate ID, missing ID/source/root/include/format, unknown decoder, missing root, inaccessible root, and invalid glob. Place one valid input first and assert its file is absent from `indexed_files` after every failing case.

- [ ] **Step 2: Run RED**

Run: `go test ./cmd/backscroll -run '^TestActiveManifestsPreflightBeforeSync$'`

Expected: FAIL because `maybeAutoSync` currently continues after `ForDef`, discovery, hash, and parse errors and may sync earlier inputs.

- [ ] **Step 3: Introduce the manifest-only contract and implement deterministic preflight**

Add the three Task 2 diagnostic codes, `ReaderLookup`, `ManifestPlan`, and `ResolvedInput` to `internal/compat/types.go` with the exact signatures in Interfaces; do not add recovery or orchestration fields. Then implement:

```go
func InspectManifests(defs []input_config.LoadedDefinition, registry ReaderLookup) (ManifestPlan, *Diagnostic, error) {
    seen := map[string]string{}
    plan := ManifestPlan{}
    for _, loaded := range defs {
        def := loaded.Definition
        if !def.Active { continue }
        if prior, ok := seen[def.ID]; ok {
            return ManifestPlan{}, invalidManifest(def.ID, fmt.Sprintf("duplicate id in %s and %s", prior, loaded.File)), nil
        }
        seen[def.ID] = loaded.File
        reader, err := registry.ForDef(def)
        if err != nil { return ManifestPlan{}, unknownDecoder(def.ID, def.Decode.Format), nil }
        if d := inspectRootAndPatterns(def); d != nil { return ManifestPlan{}, d, nil }
        plan.Inputs = append(plan.Inputs, ResolvedInput{Definition: def, ReaderName: reader.Name()})
    }
    return plan, nil, nil
}
```

`NewDefaultRegistry` registers `OpenCodeReader`, `ClaudeReader`, `PiReader`, and Task 3’s `MarkdownSectionsReader` exactly once. `DiscoverFiles` returns errors for invalid/inaccessible declared roots and unsafe patterns instead of `continue`; valid empty roots still return an empty result.

- [ ] **Step 4: GREEN and triangulate deterministic precedence**

Run: `go test ./internal/compat ./internal/input_config ./cmd/backscroll -run '^(TestActiveManifestsPreflightBeforeSync|TestManifestDiagnosticsFollowFileAndInputOrder|TestExistingRootWithNoMatchesIsValid|TestMissingDeclaredRootBlocks)$'`

Expected: PASS; the first diagnostic follows sorted manifest path then declaration order, and no `SyncFiles` call occurs before all active definitions pass.

- [ ] **Step 5: Refactor registry setup and run affected packages**

Run: `go test ./internal/readers ./internal/input_config ./internal/compat ./cmd/backscroll`

Expected: PASS.

- [ ] **Step 6: Commit all-input preflight with its command proof**

```bash
git add internal/compat/types.go internal/compat/manifest.go internal/compat/manifest_test.go internal/input_config/discover.go internal/readers/reader.go cmd/backscroll/sync_helpers.go cmd/backscroll/manifest_ingestion_test.go
git commit -m "fix(inputs): preflight all active manifests before sync"
```

### Task 3: Implement `markdown_sections` as a normal reader

**Files:**
- Create: `internal/readers/markdown_sections.go`
- Create: `internal/readers/markdown_sections_test.go`
- Modify: `internal/readers/reader.go:30-75`

**Interfaces:**
- Consumes: `input_config.DiscoverFiles`, `hashfile.HashFile`, `models.ParsedFile`, and `models.Message`.
- Produces: `type MarkdownSectionsReader struct{}`, with exact `SessionReader` methods `Name() string`, `Discover(input_config.InputDefinition) ([]string, error)`, `Hash(string) (string, error)`, and `Parse(string, input_config.InputDefinition) (models.ParsedFile, error)`.

- [ ] **Step 1: Write deterministic parser tests**

```go
func TestMarkdownSectionsReaderSectionsAndWholeDocument(t *testing.T) {
    tests := []struct{ name, body string; want []string }{
        {"ordered sections", "# ADR 1\nintro\n## Context\nctx\n## Decision\nuse sqlite\n", []string{"ADR 1\nintro", "Context\nctx", "Decision\nuse sqlite"}},
        {"no heading", "plain decision body\n", []string{"plain decision body"}},
        {"frontmatter", "---\nid: ADR-7\n---\n# Choice\nbody\n", []string{"Choice\nbody"}},
    }
    // Parse a temp file and compare Records[i].Content in order.
}
```

Also assert `Name() == "markdown_sections"`, deterministic SHA-256 hash, `Role == "document"`, and non-empty stable UUID derived from `path + heading byte offset`.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/readers -run '^TestMarkdownSectionsReader'`

Expected: FAIL with `undefined: MarkdownSectionsReader`.

- [ ] **Step 3: Implement the minimal reader using existing discovery**

`Discover` delegates directly to `input_config.DiscoverFiles(def.Discover)`. `Hash` delegates to `hashfile.HashFile`. `Parse` normalizes CRLF to LF, strips only a leading YAML frontmatter block delimited by exact `---` lines, treats ATX headings `#` through `######` as section boundaries, preserves body order, trims outer whitespace, and emits one whole-document record only when no non-empty heading section exists.

```go
func (r *MarkdownSectionsReader) Name() string { return "markdown_sections" }
func (r *MarkdownSectionsReader) Discover(def input_config.InputDefinition) ([]string, error) {
    return input_config.DiscoverFiles(def.Discover)
}
func (r *MarkdownSectionsReader) Hash(path string) (string, error) { return hashfile.HashFile(path) }
```

- [ ] **Step 4: GREEN and triangulate nested discovery/new file behavior**

Run: `go test ./internal/readers -run '^(TestMarkdownSectionsReaderSectionsAndWholeDocument|TestMarkdownSectionsReaderDiscoversNestedMarkdown|TestMarkdownSectionsReaderHashChangesForNewContent|TestMarkdownSectionsReaderIgnoresTemplateExcludes)$'`

Expected: PASS; nested `**/*.md` discovery uses the existing pipeline and adding a file changes only the discovered set, not parser ordering for existing files.

- [ ] **Step 5: Refactor section scanning and verify the interface**

Run: `go test ./internal/readers`

Expected: PASS, including `var _ SessionReader = (*MarkdownSectionsReader)(nil)`.

- [ ] **Step 6: Commit the reader and registration**

```bash
git add internal/readers/markdown_sections.go internal/readers/markdown_sections_test.go internal/readers/reader.go
git commit -m "feat(readers): decode markdown into ordered sections"
```

### Task 4: Parse every planned file before one sync and repair the decisions preset

**Files:**
- Modify: `cmd/backscroll/sync_helpers.go:16-160`
- Modify: `cmd/backscroll/manifest_ingestion_test.go`
- Modify: `inputs/decisions.inputs.toml`
- Create: `tests/fixtures/decisions/root.md`
- Create: `tests/fixtures/decisions/nested/decision.md`

**Interfaces:**
- Consumes: `preflightInputs`, `readers.NewDefaultRegistry`, existing `Database.SyncFiles([]storage.IndexedFile) error`.
- Produces: parse-complete `func collectIndexedFiles(plan compat.ManifestPlan, registry *readers.Registry, existingHashes map[string]string) ([]storage.IndexedFile, error)` and repaired decision preset instructions.

- [ ] **Step 1: Write the end-to-end retrieval test first**

`TestDecisionManifestMarkdownSectionsEndToEnd` installs an active copy of `inputs/decisions.inputs.toml` under temporary `BACKSCROLL_CONFIG_DIR`, rewrites its root to `tests/fixtures/decisions`, runs `search --text "nested-decision-sentinel" --all-projects --json`, adds `new/deeper/late.md`, runs the same command again, and asserts both records have source `decision`. It sets HOME/config/database to temporary paths.

- [ ] **Step 2: Run RED**

Run: `go test ./cmd/backscroll -run '^TestDecisionManifestMarkdownSectionsEndToEnd$'`

Expected: FAIL because `markdown_sections` is not used by the current ad hoc registry and the preset says `backscroll sync`, which is not a real command.

- [ ] **Step 3: Collect all parses before mutating and repair instructions**

Split `maybeAutoSync` into: load raw config and reject legacy sources; load all manifests; preflight all active definitions; discover/hash/parse every changed reference into memory; return on any error; call `SyncFiles` only after the complete collection succeeds. Keep existing project identification, tags, stale extraction, template, and correction derivation behavior after the preflight boundary.

Change the preset comment from `Then: backscroll sync` to executable instructions:

```text
# Verify: backscroll config
# Index and query: backscroll search --text "decision" --source decision --all-projects
```

Keep `format = "markdown_sections"` and activate the installed test copy, not necessarily the repository example, inside tests.

- [ ] **Step 4: GREEN and triangulate parse failure/no partial refresh**

Run: `go test ./cmd/backscroll -run '^(TestDecisionManifestMarkdownSectionsEndToEnd|TestParseFailurePreventsEveryInputSync|TestDiscoveryFailurePreventsCachedSearch|TestDirectReadRemainsAvailableButClaimsNoIndexFreshness)$'`

Expected: PASS; introducing one unreadable Markdown file leaves the preexisting database byte/row state unchanged and blocks the indexed query.

- [ ] **Step 5: Refactor collection into focused helpers and run affected packages**

Run: `go test ./internal/input_config ./internal/readers ./internal/compat ./cmd/backscroll`

Expected: PASS.

- [ ] **Step 6: Commit the ingestion transaction boundary and preset**

```bash
git add cmd/backscroll/sync_helpers.go cmd/backscroll/manifest_ingestion_test.go inputs/decisions.inputs.toml tests/fixtures/decisions
git commit -m "feat(inputs): ingest decision markdown after complete preflight"
```

### Task 5: Prove the ingestion half of #31 and close #33

**Files:**
- Modify: `internal/compat/manifest_test.go`
- Modify: `internal/readers/markdown_sections_test.go`
- Modify: `cmd/backscroll/manifest_ingestion_test.go`

**Interfaces:**
- Consumes: all Plan 2 behavior plus Plan 3’s active stale-index policy and executable continuation contract.
- Produces: complete #31 ingestion-half and #33 closure evidence.

- [ ] **Step 1: Run the named ingestion evidence**

Run: `go test ./internal/compat ./internal/readers ./cmd/backscroll -run '^(TestLegacySourcesRejectedWithExactManifestExample|TestActiveManifestsPreflightBeforeSync|TestMarkdownSectionsReaderSectionsAndWholeDocument|TestDecisionManifestMarkdownSectionsEndToEnd)$'`

Expected: PASS with hermetic HOME/config/database paths.

- [ ] **Step 2: Run the complete #31 gate across Plans 1 and 2**

Run: `go test ./internal/compat ./internal/storage ./internal/readers ./cmd/backscroll -run '^(TestCheckedInReleaseSchemaManifestIsComplete|TestPublishedGoLineagesUpgradeLosslessly|TestHistoricalLineageWithoutSourceMetadataUpgradesLosslessly|TestMigrationSnapshotAndRollbackOnDestructiveFailure|TestStaleIndexBlocksIndexBackedCommands|TestDirectReadRemainsAvailableButClaimsNoIndexFreshness|TestBlockingDiagnosticsHaveExecutableContinuations|TestLegacySourcesRejectedWithExactManifestExample|TestActiveManifestsPreflightBeforeSync|TestDecisionManifestMarkdownSectionsEndToEnd)$'`

Expected: PASS with no skipped tests. Plan 2 never executes before Plan 3, so `recover` is registered and functional at this boundary.

- [ ] **Step 3: Run repository gates**

Run: `just check`

Expected: PASS.

Run: `just test`

Expected: PASS.

Run: `just ci`

Expected: PASS.

- [ ] **Step 4: Record issue closure evidence exactly**

The implementation PR states: #33 closes only because `TestActiveManifestsPreflightBeforeSync` and `TestDecisionManifestMarkdownSectionsEndToEnd` pass; existing glob tests alone are insufficient. #31 closes only when every named Plan 1 migration test, every Plan 3 blocking/direct-read/executable-continuation test, and the three ingestion tests pass with no skipped case.

- [ ] **Step 5: Commit only if closure assertions changed**

```bash
git add internal/compat/manifest_test.go internal/readers/markdown_sections_test.go cmd/backscroll/manifest_ingestion_test.go
git commit -m "test(inputs): prove manifest-only decision ingestion"
```
