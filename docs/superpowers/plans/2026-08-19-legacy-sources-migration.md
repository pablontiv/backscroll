# Legacy Sources Migration and Markdown Inputs Implementation Plan

> **For implementer:** REQUIRED SUB-SKILL: Use `test-driven-development` for every behavior change. Execute in the isolated worktree `/Users/Shared/harness/.worktrees/backscroll-issue-31` on branch `fix/issue-31-upgrade-sources-migration`. Commit each task independently. After each task, run an independent requirements/code-quality review before starting the next task.

**Goal:** Make upgrades with legacy `[sources]` fail loudly before every executable command, while providing working declarative `markdown_document` and `markdown_sections` readers as the supported migration destination.

**Architecture:** A config-layer validator inspects global and local TOML files for the table's presence and returns a structured deterministic migration error. A root Cobra `PersistentPreRunE` invokes it before any command side effects. Two format-specific Markdown readers reuse the existing source parsers and enter the normal reader registry, incremental hash, `SyncFiles`, and FTS path.

**Tech Stack:** Go, Cobra, go-toml/v2, picokit/hashfile, SQLite/FTS5 integration tests, stdlib `testing`.

**Design:** `docs/superpowers/specs/2026-08-19-legacy-sources-migration-design.md`

---

## Global execution rules

- Follow red-green-refactor for each task: add a failing test, run it and observe the expected failure, implement the minimum behavior, rerun the focused tests, then commit.
- Keep tests hermetic with `t.Setenv("HOME", t.TempDir())`, `t.Setenv("BACKSCROLL_CONFIG_DIR", ...)`, `t.Setenv("BACKSCROLL_DATABASE_PATH", ...)`, and `t.Chdir(...)` where relevant.
- Do not alter migrations. The `source_metadata` lineage is already fixed by #34.
- Do not restore the legacy `sources.ParseAll` production path.
- Do not add a `sync` command or auto-generate manifests.
- Preserve unrelated work and do not implement issue #33 beyond behavior required to make the approved #31 migration target functional.

## Task 1: Reject legacy source configuration deterministically

**Files:**
- Create: `internal/config/legacy_sources.go`
- Create: `internal/config/legacy_sources_test.go`
- Reference: `internal/config/config.go`

### Step 1: Write the failing validator tests

Add table-driven tests covering:

```go
func TestValidateNoLegacySourcesReportsAllConfigFiles(t *testing.T)
func TestValidateNoLegacySourcesRejectsEmptyTable(t *testing.T)
func TestValidateNoLegacySourcesIgnoresMissingFiles(t *testing.T)
func TestValidateNoLegacySourcesReportsMalformedTOML(t *testing.T)
```

Use an isolated HOME for the global file and `t.Chdir` for `./backscroll.toml`. Assert:

- global and local files are both included, in that order;
- configured keys are sorted as `backlog, decisions, ke, memories, rules, specs`;
- an empty table reports `keys: none`;
- the caller-supplied inputs directory appears in the message;
- `markdown_document` and `markdown_sections` appear in the message;
- missing files succeed;
- malformed TOML reports its path.

Use `errors.As` to assert a typed `*LegacySourcesError` and inspect its structured entries rather than testing only substrings.

### Step 2: Run the focused tests and observe failure

```bash
go test ./internal/config -run 'TestValidateNoLegacySources' -count=1
```

Expected: compile failure because `ValidateNoLegacySources` and `LegacySourcesError` do not exist.

### Step 3: Implement the minimum validator

In `internal/config/legacy_sources.go`, add:

```go
type LegacySourcesFile struct {
    Path string
    Keys []string
}

type LegacySourcesError struct {
    Files     []LegacySourcesFile
    InputsDir string
}

func (e *LegacySourcesError) Error() string
func ValidateNoLegacySources(inputsDir string) error
```

Implementation requirements:

- inspect `globalConfigPath()` then `./backscroll.toml`;
- ignore `os.IsNotExist` only;
- unmarshal a minimal envelope with `Sources *SourcesConfig` so an empty table is detectable;
- collect every offending file before returning;
- derive only non-empty known keys;
- sort keys lexicographically;
- produce one deterministic message with migration mappings and no terminal styling;
- wrap read/TOML errors with the path.

Do not call `sources.ParseAll`, mutate `Config`, or touch the database.

### Step 4: Run focused and package tests

```bash
go test ./internal/config -run 'TestValidateNoLegacySources' -count=1
go test ./internal/config -count=1
```

Expected: PASS.

### Step 5: Commit

```bash
git add internal/config/legacy_sources.go internal/config/legacy_sources_test.go
git commit -m "fix(config): reject legacy sources tables"
```

### Task 1 review checkpoint

An independent reviewer must verify:

- table presence, not merely non-empty values, triggers rejection;
- both config locations are reported;
- ordering and message output are deterministic;
- missing files are the only ignored read error;
- no ingestion or DB behavior was added.

Resolve all confirmed findings and obtain re-review before Task 2.

## Task 2: Enforce the migration boundary before every command

**Files:**
- Modify: `cmd/backscroll/main.go`
- Create: `cmd/backscroll/legacy_sources_test.go`
- Reference: `cmd/backscroll/read.go`
- Reference: `cmd/backscroll/recover.go`
- Reference: `cmd/backscroll/config.go`
- Reference: `internal/input_config/loader.go`

### Step 1: Write failing CLI preflight tests

Create helpers that write a legacy global config under the isolated HOME and execute `buildRootCmd` directly.

Add:

```go
func TestLegacySourcesBlockEveryExecutableCommand(t *testing.T)
func TestLegacySourcesPreflightHasNoSideEffects(t *testing.T)
func TestLegacySourcesAllowsHelpAndVersion(t *testing.T)
```

Table-test these syntactically valid invocations:

```text
search
read --path <missing-path>
list
patterns --kind commands
rebuild
purge --before 2026-01-01
validate --indexed-only
status
config
annotate --uuid test-uuid --kind correction --label false-positive
recover --from <missing-path>
```

For every executable command, assert the returned error is or wraps `*config.LegacySourcesError`. For side effects, point `BACKSCROLL_DATABASE_PATH` at a nonexistent temp path and assert it remains absent; also assert `read` and `recover` fail with the migration error rather than their missing-path errors.

Verify these remain successful and contain normal help/version output:

```text
--help
--version
search --help
```

### Step 2: Run the focused tests and observe failure

```bash
go test ./cmd/backscroll -run 'TestLegacySources' -count=1
```

Expected: executable commands proceed past root dispatch; at least `read` returns its missing-file error instead of the migration error.

### Step 3: Add the root preflight

In `buildRootCmd`, add a `PersistentPreRunE` that:

```go
inputsDir, err := input_config.InputsDir()
if err != nil {
    return fmt.Errorf("resolve inputs directory: %w", err)
}
return config.ValidateNoLegacySources(inputsDir)
```

Import `internal/config` and `internal/input_config` in `main.go`. Do not open the database, load active inputs, or duplicate validator formatting in the command package.

### Step 4: Run focused and command tests

```bash
go test ./cmd/backscroll -run 'TestLegacySources' -count=1
go test ./cmd/backscroll -count=1
```

Expected: PASS.

### Step 5: Commit

```bash
git add cmd/backscroll/main.go cmd/backscroll/legacy_sources_test.go
git commit -m "fix(cli): block commands with legacy sources"
```

### Task 2 review checkpoint

An independent reviewer must verify:

- every executable subcommand inherits the hook;
- argument sets are valid enough to reach the hook;
- the hook runs before database, source-file, and recovery effects;
- help/version remain usable;
- errors preserve the typed config error with `%w` where wrapping occurs.

Resolve all confirmed findings and obtain re-review before Task 3.

## Task 3: Add whole-document and sectioned Markdown readers

**Files:**
- Create: `internal/readers/markdown_reader.go`
- Create: `internal/readers/markdown_reader_test.go`
- Modify: `cmd/backscroll/sync_helpers.go`
- Modify: `cmd/backscroll/main_test.go` or create `cmd/backscroll/markdown_registry_test.go`
- Reference: `internal/readers/reader.go`
- Reference: `internal/sources/sources.go`
- Reference: `internal/input_config/discover.go`

### Step 1: Write failing reader tests

Add tests:

```go
func TestMarkdownDocumentReaderParse(t *testing.T)
func TestMarkdownSectionsReaderParse(t *testing.T)
func TestMarkdownSectionsReaderFallsBackToDocument(t *testing.T)
func TestMarkdownReaderDiscoverAndHash(t *testing.T)
func TestMarkdownReaderReportsPathErrors(t *testing.T)
```

Use fixed file modtimes with `os.Chtimes`. Assert:

- format names are exactly `markdown_document` and `markdown_sections`;
- document mode returns one record containing the full document;
- section mode returns one record per `## ` section using existing parser semantics;
- no-section mode returns one trimmed whole-document record;
- `Role == "document"`, `ContentType == "text"`, and `UUID == ""`;
- every record uses the file modification time;
- `ParsedFile.Path`, SHA-256 `Hash`, and parent-directory `Cwd` are correct;
- discovery honors roots, includes, and excludes;
- missing file errors include the path.

Add a failing command-package test:

```go
func TestDefaultAutoSyncRegistryIncludesMarkdownReaders(t *testing.T)
```

Resolve both formats through `newDefaultAutoSyncRegistry().ForDef(...)`.

### Step 2: Run the focused tests and observe failure

```bash
go test ./internal/readers ./cmd/backscroll -run 'TestMarkdown|TestDefaultAutoSyncRegistryIncludesMarkdown' -count=1
```

Expected: compile failure because Markdown readers do not exist, or registry lookup failure for both formats.

### Step 3: Implement shared Markdown parsing

In `internal/readers/markdown_reader.go`, provide two constructors or concrete readers with stable names. Share private logic to avoid duplicating discovery, hashing, stat, and source-item mapping.

Requirements:

- `Discover` calls `input_config.DiscoverFiles`;
- `Hash` calls `hashfile.HashFile`;
- `Parse` hashes and stats the path, then calls `sources.ParseDocument` or `sources.ParseSectioned` with `def.Source`;
- each item maps to one `models.Message` with approved role/content type/modtime/empty UUID;
- returned CWD is `filepath.Dir(path)`;
- contextual errors include the operation and path.

Do not parse structured frontmatter into messages, generate UUIDs, or add storage code.

### Step 4: Register both formats

In `newDefaultAutoSyncRegistry`, register the two Markdown reader instances alongside the three existing readers.

### Step 5: Run focused, reader, and command tests

```bash
go test ./internal/readers -count=1
go test ./cmd/backscroll -run 'TestDefaultAutoSyncRegistryIncludesMarkdown|TestMaybeAutoSync' -count=1
go test ./cmd/backscroll -count=1
```

Expected: PASS.

### Step 6: Commit

```bash
git add internal/readers/markdown_reader.go internal/readers/markdown_reader_test.go cmd/backscroll/sync_helpers.go cmd/backscroll/markdown_registry_test.go
git commit -m "feat(readers): support declarative markdown inputs"
```

Adjust the final `git add` file list if the registry test is placed in an existing test file.

### Task 3 review checkpoint

An independent reviewer must verify:

- the reader uses existing source parser semantics rather than introducing a second Markdown parser;
- hash/modtime/path behavior is deterministic;
- source classification remains `def.Source` in `maybeAutoSync`;
- empty UUID correctly retains mutable non-session wipe-and-reload behavior;
- no unsupported frontmatter claims or invented metadata appear.

Resolve all confirmed findings and obtain re-review before Task 4.

## Task 4: Prove declarative ingestion and publish migration guidance

**Files:**
- Create: `cmd/backscroll/markdown_inputs_test.go`
- Modify: `inputs/decisions.inputs.toml`
- Modify: `CLAUDE.md`
- Modify if needed: `internal/config/legacy_sources.go`

### Step 1: Write the failing end-to-end test

Add:

```go
func TestDeclarativeMarkdownSectionsAutoSyncAndSearch(t *testing.T)
```

Hermetic setup:

1. isolate HOME and working directory;
2. set `BACKSCROLL_CONFIG_DIR` and `BACKSCROLL_DATABASE_PATH`;
3. create `<config-dir>/backscroll/inputs/decisions.inputs.toml` with one active input, an absolute root, `source = "decision"`, `include = ["**/*.md"]`, and `decode.format = "markdown_sections"`;
4. create a decision Markdown file with two `## ` sections and a unique search term;
5. execute the real command path with `buildRootCmd` and `search --all-projects --source decision <unique-term>` (use the command's actual query flag syntax);
6. assert the expected section and source are returned;
7. execute the same command again and assert indexed row/file counts are unchanged.

Use direct DB queries only for the idempotency counts. Do not mock the registry, discovery, parser, or `SyncFiles`.

### Step 2: Run the focused test and observe the expected state

```bash
go test ./cmd/backscroll -run 'TestDeclarativeMarkdownSectionsAutoSyncAndSearch' -count=1
```

If Task 3 already makes the behavior pass, record that the test is characterization coverage added after the unit-level red-green implementation; do not introduce unnecessary production changes merely to force a failure.

### Step 3: Correct the shipped preset

Update `inputs/decisions.inputs.toml` comments:

- keep `active = false`;
- remove `backscroll sync`;
- explain automatic sync before index-consuming commands;
- state that sections headed by `## ` are indexed;
- remove claims that YAML frontmatter is parsed into structured metadata.

### Step 4: Update project documentation

In `CLAUDE.md`:

- update the implemented/readers descriptions to include `markdown_document` and `markdown_sections`;
- update the Module Layout readers line;
- update the Package Layout readers line;
- document the legacy `[sources]` hard-error migration boundary and the key-to-format mapping in Key Design Decisions;
- keep the migration count at v13 and do not describe a schema change.

### Step 5: Run focused regressions

```bash
go test ./cmd/backscroll -run 'TestDeclarativeMarkdownSectionsAutoSyncAndSearch|TestLegacySources' -count=1
go test ./internal/compat -run 'TestInspectIndexUsesObservedShapeNotVersionAlone' -count=1
go test ./internal/storage -run 'TestMigration.*NoSourceMetadata|TestMigrationPlan' -count=1
```

If the final storage regex matches no exact test name, run the relevant `internal/storage` migration-plan tests by their actual names discovered with `go test ./internal/storage -list 'Migration'`.

Expected: PASS, including the #34 no-`source_metadata` behavior.

### Step 6: Commit

```bash
git add cmd/backscroll/markdown_inputs_test.go inputs/decisions.inputs.toml CLAUDE.md
git commit -m "docs(config): guide markdown source migration"
```

Include any tightly related test-helper adjustment in the same commit and document why.

### Task 4 review checkpoint

An independent reviewer must verify:

- the test crosses manifest loading, discovery, reader selection, sync, and source-filtered search;
- the second run proves hash-based idempotency;
- the preset does not promise nonexistent commands or frontmatter semantics;
- CLAUDE.md package/module inventories remain accurate;
- issue #31's already-fixed migration half is not reimplemented.

Resolve all confirmed findings and obtain re-review before final verification.

## Task 5: Whole-branch review and verification

**Files:**
- Review: all changes in `origin/main...HEAD`
- Modify: only files required by confirmed review findings

### Step 1: Run an independent whole-branch review

Review against:

- issue #31;
- approved design;
- this implementation plan;
- project coverage and hermetic-test requirements;
- the constraint that every issue ships through its own PR.

Classify findings as blocking, important, or optional. Fix confirmed blocking/important findings, then obtain independent re-review.

### Step 2: Run formatting and static checks

```bash
just check
git diff --check origin/main...HEAD
```

Expected: PASS and no whitespace errors.

### Step 3: Run the full test and CI gates fresh

```bash
just test
just ci
```

Expected: PASS with aggregate statement coverage at or above 85%.

### Step 4: Inspect final branch scope

```bash
git status --short --branch
git log --oneline origin/main..HEAD
git diff --stat origin/main...HEAD
git diff --name-status origin/main...HEAD
```

Expected:

- clean working tree;
- only #31 design, plan, config preflight, Markdown readers, tests, preset, and CLAUDE documentation;
- no #35 commits or unrelated harness work;
- branch based directly on `origin/main`.

### Step 5: Commit any final review fixes

If review required changes:

```bash
git add <confirmed-fix-files>
git commit -m "fix: address legacy sources review findings"
```

Then rerun Steps 2–4. Never claim completion from cached or pre-fix output.

### Step 6: Prepare the dedicated PR

Push only after verification and open a dedicated PR for #31. The PR body must:

- use the repository template;
- include `Fixes #31`;
- state that #34 already resolved the missing-`source_metadata` migration lineage;
- explain that this PR rejects legacy `[sources]` and makes both Markdown manifest formats functional;
- include exact `just check` and `just ci` evidence;
- mention that #33 will be classified separately after this PR.
