# Mandatory Sync and Database-Only Retrieval Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore SQLite as Backscroll's only public retrieval source and run one mandatory incremental sync before every operational command, with `recover` as the sole controlled continuation after startup failure.

**Architecture:** Cobra's root `PersistentPreRunE` owns configuration validation, manifest preflight, compatibility preparation, and one incremental sync attempt. It stores the prepared configuration or typed startup failure in command context; handlers open the already-synchronized index without choosing freshness, while `recover` alone may consume a startup failure and perform a post-install sync.

**Tech Stack:** Go 1.x, Cobra, modernc.org/sqlite, stdlib `testing`, existing `internal/config`, `internal/input_config`, `internal/storage`, and `internal/recovery` packages.

**Spec:** `docs/superpowers/specs/2026-08-20-mandatory-sync-db-only-retrieval-design.md`

## Global Constraints

- SQLite is the only public retrieval source; transient files are ingestion inputs only.
- Every operational command attempts incremental startup sync exactly once before its handler runs.
- `recover` is the only command allowed to continue after startup failure.
- `--help` and `--version` must not load configuration, open SQLite, or sync inputs.
- Configuration, manifest, compatibility, discovery, parse, and sync failures fail closed without cached rows.
- JSON and robot stdout must remain parseable; human progress and warnings go to stderr.
- `backscroll read` and every `--indexed-only` flag are removed without aliases or deprecated no-op registrations.
- `search --source-path` remains the database-backed path lookup.
- Recovery must verify the installed canonical database and run a post-install sync before reporting success.
- No daemon, watcher, public `sync` command, snapshot bypass, FTS ranking change, or schema migration is introduced.
- Tests must scrub `HOME` and `BACKSCROLL_CONFIG_DIR`; the release gate is aggregate statement coverage >=85%.

---

## File Structure

- `cmd/backscroll/startup_policy.go`: root startup orchestration, injectable policy function, command-context state, typed failure rendering, recovery exception.
- `cmd/backscroll/startup_policy_test.go`: command-tree invariants, exactly-once ordering, fail-closed behavior, help/version side-effect checks, machine-output checks.
- `cmd/backscroll/main.go`: wire the root policy and remove `read` registration.
- `cmd/backscroll/index_policy.go`: retain compatibility/opening primitives but remove handler-selected `autoSync`.
- `cmd/backscroll/sync_helpers.go`: keep incremental ingestion implementation; accept an explicit progress writer instead of relying on freshness choices in handlers.
- `cmd/backscroll/{search,list,patterns,status,validate,rebuild,purge,annotate,config}.go`: consume startup configuration and open the synchronized index without syncing.
- `cmd/backscroll/recover.go`: consume startup failure context, execute recovery, then sync the installed database before success output.
- `cmd/backscroll/read.go`: delete.
- `internal/reader/`: delete after production-consumer check.
- `cmd/backscroll/*_test.go`: replace snapshot bypass fixtures with hermetic startup inputs and update function signatures.
- `README.md`, `CLAUDE.md`, `docs/{audit-integration,configuration,input-contract,patterns,read,sync}.md`, `.claude/skills/backscroll/{SKILL,ref-context-mode}.md`: publish the mandatory manifest -> sync -> SQLite -> query contract.
- Historical files under `docs/roadmap/` and older `docs/superpowers/` remain unchanged; ADR 0002 already records their supersession.

---

### Task 1: Lock the Public CLI Invariants

**Files:**
- Create: `cmd/backscroll/startup_policy_test.go`
- Modify: `cmd/backscroll/main.go`
- Delete: `cmd/backscroll/read.go`
- Delete: `internal/reader/reader.go`
- Delete: `internal/reader/semantic.go`
- Delete: `internal/reader/reader_test.go`
- Delete: `internal/reader/semantic_test.go`

**Interfaces:**
- Consumes: `buildRootCmd(stdout, stderr io.Writer) *cobra.Command`.
- Produces: a root command tree with no `read` command and no flag named `indexed-only` anywhere.

- [ ] **Step 1: Write failing command-tree invariant tests**

```go
func TestRootExcludesDirectReadCommand(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	for _, cmd := range root.Commands() {
		if cmd.Name() == "read" {
			t.Fatal("public read command is registered")
		}
	}
}

func TestCommandTreeExcludesIndexedOnlyFlag(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if flag := cmd.Flags().Lookup("indexed-only"); flag != nil {
			t.Errorf("%s registers forbidden --indexed-only", cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}
```

- [ ] **Step 2: Run the invariant tests and verify they fail**

Run: `go test ./cmd/backscroll -run 'TestRootExcludesDirectReadCommand|TestCommandTreeExcludesIndexedOnlyFlag' -count=1`

Expected: FAIL because `read` and `--indexed-only` are currently registered.

- [ ] **Step 3: Remove `newReadCmd` from `root.AddCommand` and delete the direct-reader files**

```go
root.AddCommand(
	newSearchCmd(stdout, stderr),
	newListCmd(stdout, stderr),
	newPatternsCmd(stdout, stderr),
	newRebuildCmd(stdout, stderr),
	newPurgeCmd(stdout, stderr),
	newValidateCmd(stdout, stderr),
	newStatusCmd(stdout, stderr),
	newConfigCmd(stdout, stderr),
	newAnnotateCmd(stdout, stderr),
	newRecoverCmd(stdout, stderr),
)
```

Run: `rm cmd/backscroll/read.go internal/reader/reader.go internal/reader/semantic.go internal/reader/reader_test.go internal/reader/semantic_test.go`

- [ ] **Step 4: Remove all five public flag registrations and parameters**

Remove `indexedOnly` variables, `BoolVar` calls, help text, and function parameters from `search.go`, `list.go`, `patterns.go`, `status.go`, and `validate.go`. The resulting calls must have these shapes:

```go
return runSearch(stdout, stderr, query, project, allProjects, jsonFormat, robotFormat,
	source, sourcePath, after, before, role, limit, offset, contentType, tag,
	fields, maxTokens, lexicalOnly, similarityThreshold)

return runList(stdout, stderr, project, allProjects, recent, jsonFormat, robotFormat,
	order, limit, offset)

return runPatterns(stdout, stderr, kind, project, allProjects, tag, limit, offset,
	jsonFormat, robotFormat, minSupport, minConfidence, pending, batch,
	minLength, maxLength, after, before, trend)

return runStatus(stdout, stderr, jsonFormat)
return runValidate(stdout, stderr, jsonFormat)
```

- [ ] **Step 5: Run the invariant tests and compilation check**

Run: `go test ./cmd/backscroll -run 'TestRootExcludesDirectReadCommand|TestCommandTreeExcludesIndexedOnlyFlag' -count=1`

Expected: package compilation may still fail at stale test call sites, but neither failure may report a remaining production `read` or `indexed-only` registration.

- [ ] **Step 6: Commit the public-surface removal**

```bash
git add cmd/backscroll/main.go cmd/backscroll/search.go cmd/backscroll/list.go cmd/backscroll/patterns.go cmd/backscroll/status.go cmd/backscroll/validate.go cmd/backscroll/startup_policy_test.go
git add -u cmd/backscroll/read.go internal/reader
git commit -m "feat(cli): remove direct and stale retrieval bypasses"
```

---

### Task 2: Centralize Mandatory Startup Policy

**Files:**
- Create: `cmd/backscroll/startup_policy.go`
- Modify: `cmd/backscroll/main.go`
- Modify: `cmd/backscroll/index_policy.go`
- Modify: `cmd/backscroll/sync_helpers.go`
- Test: `cmd/backscroll/startup_policy_test.go`
- Test: `cmd/backscroll/index_policy_test.go`

**Interfaces:**
- Consumes: `config.Load()`, `config.ValidateNoLegacySources(string)`, `input_config.ActiveInputs([]string)`, `prepareIndex(context.Context, *config.Config, indexCommandClass)`, and `maybeAutoSync(*config.Config, io.Writer)`.
- Produces:
  - `type startupPolicyFunc func(context.Context, io.Writer) startupResult`
  - `type startupResult struct { Config *config.Config; Diagnostic *compat.Diagnostic; Err error }`
  - `var startupSync = maybeAutoSync`
  - `func defaultStartupPolicy(context.Context, io.Writer) startupResult`
  - `func buildRootCmdWithStartup(io.Writer, io.Writer, startupPolicyFunc) *cobra.Command`
  - `func startupResultFrom(*cobra.Command) startupResult`
  - `func prepareIndex(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error)`
  - `func maybeAutoSync(*config.Config, io.Writer) error`

- [ ] **Step 1: Write failing exactly-once and ordering tests using an injected policy**

```go
func TestEveryOperationalCommandRunsStartupBeforeHandler(t *testing.T) {
	commands := [][]string{
		{"search", "needle"}, {"list"}, {"patterns", "--kind", "commands"},
		{"annotate", "--uuid", "u", "--kind", "correction", "--label", "x"},
		{"purge", "--before", "2030-01-01"}, {"rebuild"}, {"status"},
		{"validate"}, {"config"}, {"recover", "--from", "missing.db", "--dry-run"},
	}
	for _, argv := range commands {
		t.Run(strings.Join(argv, "_"), func(t *testing.T) {
			calls := 0
			policy := func(context.Context, io.Writer) startupResult {
				calls++
				return startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}}
			}
			root := buildRootCmdWithStartup(io.Discard, io.Discard, policy)
			root.SetArgs(argv)
			_ = root.Execute()
			if calls != 1 {
				t.Fatalf("startup calls=%d, want 1", calls)
			}
		})
	}
}
```

Add a focused synthetic command in the same test that appends `"startup"` inside the policy and `"handler"` inside `RunE`, then assert `[]string{"startup", "handler"}`.

Also exercise the production policy with an injected sync seam:

```go
func TestDefaultStartupPolicyCallsSyncExactlyOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	calls := 0
	originalSync := startupSync
	startupSync = func(*config.Config, io.Writer) error {
		calls++
		return nil
	}
	t.Cleanup(func() { startupSync = originalSync })

	result := defaultStartupPolicy(context.Background(), io.Discard)
	if result.Err != nil || result.Diagnostic != nil {
		t.Fatalf("startup result=%+v", result)
	}
	if calls != 1 {
		t.Fatalf("sync calls=%d, want 1", calls)
	}
}
```

- [ ] **Step 2: Write failing help/version side-effect tests**

```go
func TestMetadataCommandsSkipStartup(t *testing.T) {
	for _, argv := range [][]string{{"--help"}, {"--version"}, {"search", "--help"}} {
		calls := 0
		root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
			calls++
			return startupResult{}
		})
		root.SetArgs(argv)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", argv, err)
		}
		if calls != 0 {
			t.Fatalf("%v invoked startup %d times", argv, calls)
		}
	}
}
```

- [ ] **Step 3: Run the startup tests and verify they fail**

Run: `go test ./cmd/backscroll -run 'TestEveryOperationalCommandRunsStartupBeforeHandler|TestMetadataCommandsSkipStartup' -count=1`

Expected: FAIL because `buildRootCmdWithStartup`, `startupResult`, and centralized orchestration do not exist.

- [ ] **Step 4: Implement startup state and root wiring**

```go
type startupPolicyFunc func(context.Context, io.Writer) startupResult

type startupResult struct {
	Config     *config.Config
	Diagnostic *compat.Diagnostic
	Err        error
}

type startupContextKey struct{}

func startupResultFrom(cmd *cobra.Command) startupResult {
	result, _ := cmd.Context().Value(startupContextKey{}).(startupResult)
	return result
}

func buildRootCmd(stdout, stderr io.Writer) *cobra.Command {
	return buildRootCmdWithStartup(stdout, stderr, defaultStartupPolicy)
}
```

In `buildRootCmdWithStartup`, set `PersistentPreRunE` to call the policy once, store its result with `cmd.SetContext(context.WithValue(cmd.Context(), startupContextKey{}, result))`, return nil on success, allow only `cmd.Name() == "recover"` to continue on failure, and otherwise render/return the typed failure before any handler output.

- [ ] **Step 5: Implement default preflight ordering**

```go
func defaultStartupPolicy(ctx context.Context, progress io.Writer) startupResult {
	inputsDir, err := input_config.InputsDir()
	if err != nil {
		return startupResult{Err: fmt.Errorf("resolve inputs directory: %w", err)}
	}
	if err := config.ValidateNoLegacySources(inputsDir); err != nil {
		return startupResult{Err: err}
	}
	cfg, err := config.Load()
	if err != nil {
		return startupResult{Err: fmt.Errorf("load config: %w", err)}
	}
	if _, _, err := input_config.ActiveInputs(cfg.SessionDirs); err != nil {
		return startupResult{Config: cfg, Err: fmt.Errorf("validate active inputs: %w", err)}
	}
	db, diag, err := prepareIndex(ctx, cfg, indexMutation)
	if db != nil {
		err = closeIndexDB(db, err)
	}
	if diag != nil || err != nil {
		return startupResult{Config: cfg, Diagnostic: diag, Err: err}
	}
	if err := startupSync(cfg, progress); err != nil {
		activePath, _ := resolveActiveIndexPath(cfg.DatabasePath)
		d := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: fmt.Sprintf("index sync failed: %v", err)}, activePath)
		return startupResult{Config: cfg, Diagnostic: &d, Err: err}
	}
	return startupResult{Config: cfg}
}
```

- [ ] **Step 6: Remove `autoSync` from `prepareIndex`**

Change the signature to:

```go
func prepareIndex(ctx context.Context, cfg *config.Config, class indexCommandClass) (*storage.Database, *compat.Diagnostic, error)
```

Delete the database-existence snapshot branch and the close/sync/reopen block. Map both `indexDataRead` and `indexMutation` to `storage.OpenCompatible`; retain immutable openings only for internal diagnostic/remediation inspection.

- [ ] **Step 7: Make sync progress explicit**

Change:

```go
func maybeAutoSync(cfg *config.Config, progress io.Writer) (retErr error)
```

Replace both `fmt.Fprintf(maybeAutoSyncProgress, ...)` calls with `fmt.Fprintf(progress, ...)`. Keep storage/reader injection variables for deterministic failure tests, but remove the `maybeAutoSyncProgress` global.

- [ ] **Step 8: Run the focused policy tests**

Run: `go test ./cmd/backscroll -run 'TestEveryOperationalCommandRunsStartupBeforeHandler|TestMetadataCommandsSkipStartup|TestAutoSyncFailuresBlockCachedConsumers|TestResolveActiveIndexPathPropagatesBrokenSymlink' -count=1`

Expected: PASS after adapting direct `prepareIndex`/`maybeAutoSync` test calls to their new signatures.

- [ ] **Step 9: Commit centralized startup orchestration**

```bash
git add cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go cmd/backscroll/main.go cmd/backscroll/index_policy.go cmd/backscroll/index_policy_test.go cmd/backscroll/sync_helpers.go cmd/backscroll/main_test.go
git commit -m "feat(cli): enforce mandatory startup sync"
```

---

### Task 3: Make Handlers Consume the Synchronized Index

**Files:**
- Modify: `cmd/backscroll/search.go`
- Modify: `cmd/backscroll/list.go`
- Modify: `cmd/backscroll/patterns.go`
- Modify: `cmd/backscroll/status.go`
- Modify: `cmd/backscroll/validate.go`
- Modify: `cmd/backscroll/rebuild.go`
- Modify: `cmd/backscroll/purge.go`
- Modify: `cmd/backscroll/annotate.go`
- Modify: `cmd/backscroll/config.go`
- Test: `cmd/backscroll/startup_policy_test.go`
- Test: `cmd/backscroll/index_policy_test.go`

**Interfaces:**
- Consumes: `startupResultFrom(cmd).Config` and `prepareIndex(ctx, cfg, class)`.
- Produces: handlers that never call `maybeAutoSync`, never choose freshness, and execute only after root startup success.

- [ ] **Step 1: Write a failing fail-closed handler test**

```go
func TestStartupFailurePreventsHandlerOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	policyErr := errors.New("injected startup failure")
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Err: policyErr}
	})
	root.SetArgs([]string{"config", "--json"})
	err := root.Execute()
	if !errors.Is(err, policyErr) {
		t.Fatalf("error=%v, want injected startup failure", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("handler emitted output after startup failure: %q", stdout.String())
	}
}
```

- [ ] **Step 2: Run the test and verify it fails before handler rewiring is complete**

Run: `go test ./cmd/backscroll -run TestStartupFailurePreventsHandlerOutput -count=1`

Expected: FAIL if a handler remains reachable or writes output without consuming root policy.

- [ ] **Step 3: Pass root-loaded configuration into every handler**

For each command `RunE`, obtain the context value before calling its helper:

```go
startup := startupResultFrom(cmd)
if startup.Config == nil {
	return fmt.Errorf("startup configuration unavailable")
}
return runSearch(cmd.Context(), stdout, stderr, startup.Config, query, /* existing flags */)
```

Apply the equivalent signature prefix to all operational helpers:

```go
func runSearch(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, /* flags */) error
func runList(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, /* flags */) error
func runPatterns(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, /* flags */) error
func runStatus(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, jsonFormat bool) error
func runValidate(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, jsonFormat bool) error
func runRebuild(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config) error
func runPurge(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, before string) error
func runAnnotate(ctx context.Context, stdout, stderr io.Writer, cfg *config.Config, uuid, path string, ordinal int, kind, label string) error
func runConfig(stdout, stderr io.Writer, cfg *config.Config, jsonFormat bool) error
```

Delete every handler-local `config.Load()` call.

- [ ] **Step 4: Open without syncing in index-backed handlers**

Use:

```go
db, diag, err := prepareIndex(ctx, cfg, indexDataRead)
```

for `search`, `list`, and `patterns`; use `indexMutation` for `annotate`, `purge`, and `rebuild`; use `indexDiagnostic` for post-sync `status` and `validate`. No handler calls `maybeAutoSync`.

Remove the stale-snapshot no-database special case from `list`, `status`, and `validate`: startup now creates/prepares the canonical database before these handlers run.

- [ ] **Step 5: Remove rebuild's second sync claim**

Change its long help from “runs an incremental sync” to “operates on the index synchronized at command startup.” Keep FTS rebuild, derived backfill, and project re-resolution unchanged.

- [ ] **Step 6: Run command policy and focused handler tests**

Run: `go test ./cmd/backscroll -run 'TestStartupFailurePreventsHandlerOutput|TestStaleIndexBlocksIndexBackedCommands|TestRebuildFailsOnDerivedMaintenanceError' -count=1`

Expected: PASS; no cached row appears after startup failure and rebuild performs no second sync.

- [ ] **Step 7: Commit handler simplification**

```bash
git add cmd/backscroll/search.go cmd/backscroll/list.go cmd/backscroll/patterns.go cmd/backscroll/status.go cmd/backscroll/validate.go cmd/backscroll/rebuild.go cmd/backscroll/purge.go cmd/backscroll/annotate.go cmd/backscroll/config.go cmd/backscroll/startup_policy_test.go cmd/backscroll/index_policy_test.go
git commit -m "refactor(cli): consume root-synchronized index"
```

---

### Task 4: Implement Controlled Recovery Continuation

**Files:**
- Modify: `cmd/backscroll/startup_policy.go`
- Modify: `cmd/backscroll/recover.go`
- Test: `cmd/backscroll/startup_policy_test.go`
- Test: `cmd/backscroll/recover_test.go`

**Interfaces:**
- Consumes: `startupResultFrom(cmd)`, `recovery.Execute`, and `maybeAutoSync(cfg, stderr)`.
- Produces: `recover` continuation after startup failure, preservation of the original cause, and post-install sync before success output.

- [ ] **Step 1: Write a failing controlled-continuation test**

```go
func TestRecoverAloneContinuesAfterStartupFailure(t *testing.T) {
	startupErr := errors.New("injected startup failure")
	called := false
	root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}, Err: startupErr}
	})
	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		called = true
		return recovery.Report{}, errors.New("injected recovery failure")
	}
	t.Cleanup(func() { recoverExecute = originalExecute })
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err := root.Execute()
	if !called {
		t.Fatal("recover handler did not continue after startup failure")
	}
	if !errors.Is(err, startupErr) {
		t.Fatalf("error=%v does not preserve startup failure", err)
	}
}
```

Also add a table case proving `search`, `list`, and `config` do not continue under the same policy.

- [ ] **Step 2: Write a failing post-install sync ordering test**

Inject recovery and sync seams, append `"recover"`, `"sync"`, and `"report"` to a slice, and assert exactly:

```go
want := []string{"recover", "sync", "report"}
```

The report marker is captured by a writer that records its first write.

- [ ] **Step 3: Add deterministic recovery seams**

```go
var recoverExecute = recovery.Execute
var recoverPostInstallSync = maybeAutoSync
```

Use `recoverExecute` in `newRecoverCmd`. Tests restore both variables with `t.Cleanup`.

- [ ] **Step 4: Consume startup failure and join causes on failure**

At the start of `RunE`:

```go
startup := startupResultFrom(cmd)
cfg := startup.Config
if cfg == nil {
	loaded, err := config.Load()
	if err != nil {
		return errors.Join(startup.Err, fmt.Errorf("load config for recovery: %w", err))
	}
	cfg = loaded
}
```

If `recoverExecute` fails, return `errors.Join(startup.Err, fmt.Errorf("recovery failed: %w", err))`. If it succeeds, the prior startup failure is considered remediated and is not returned.

- [ ] **Step 5: Sync the installed database before success output**

```go
if !dryRun {
	if err := recoverPostInstallSync(cfg, stderr); err != nil {
		return errors.Join(startup.Err, fmt.Errorf("post-recovery sync: %w", err))
	}
}
printRecoveryReport(stdout, report, dryRun)
```

Dry runs do not replace the database and therefore do not run post-install sync.

- [ ] **Step 6: Run recovery tests**

Run: `go test ./cmd/backscroll -run 'TestRecoverAloneContinuesAfterStartupFailure|TestRecoverPostInstallSyncBeforeReport|TestRecover' -count=1`

Expected: PASS, including existing durable backup and canonical union tests.

- [ ] **Step 7: Commit recovery continuation**

```bash
git add cmd/backscroll/startup_policy.go cmd/backscroll/startup_policy_test.go cmd/backscroll/recover.go cmd/backscroll/recover_test.go
git commit -m "fix(recovery): continue after failed startup sync"
```

---

### Task 5: Prove Manifest-Backed Markdown Retrieval and Machine Output

**Files:**
- Modify: `cmd/backscroll/index_policy_test.go`
- Modify: `cmd/backscroll/diagnostics_test.go`
- Modify: `cmd/backscroll/markdown_registry_test.go`

**Interfaces:**
- Consumes: `writeInputManifest`, `run`, Markdown readers, and `search --source-path`.
- Produces: integration evidence that Markdown is ingested through manifests and retrieved only from SQLite, with clean JSON/robot output.

- [ ] **Step 1: Write a failing whole-document integration test**

```go
func TestMandatoryStartupIndexesMarkdownDocumentForSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	root := filepath.Join(t.TempDir(), "notes")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	writeInputManifest(t, root, "markdown_document", root, []string{"*.md"}, nil)
	path := filepath.Join(root, "decision.md")
	writeFile(t, path, "# Decision\n\nperennial sqlite sentinel\n")

	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"search", "perennial sqlite sentinel", "--all-projects", "--source-path", path, "--json"})
	if err != nil {
		t.Fatalf("search: %v stderr=%q", err, stderr.String())
	}
	var rows []minimalSearchResult
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	if len(rows) != 1 || rows[0].SourcePath != path {
		t.Fatalf("rows=%+v, want indexed markdown path %s", rows, path)
	}
}
```

- [ ] **Step 2: Write a sectioned Markdown integration test**

Use `format = "markdown_sections"`, a file containing two `##` sections, and search for a token unique to the second section:

```go
func TestMandatoryStartupIndexesMarkdownSectionsForSearch(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	root := filepath.Join(t.TempDir(), "notes")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	writeInputManifest(t, root, "markdown_sections", root, []string{"*.md"}, nil)
	path := filepath.Join(root, "decisions.md")
	writeFile(t, path, "# Decisions\n\n## First\nalpha\n\n## Second\nsection sentinel omega\n")

	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"search", "section sentinel omega", "--all-projects", "--source-path", path, "--json"})
	if err != nil {
		t.Fatalf("search: %v stderr=%q", err, stderr.String())
	}
	var rows []minimalSearchResult
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON %q: %v", stdout.String(), err)
	}
	if len(rows) != 1 || rows[0].SourcePath != path || !strings.Contains(rows[0].Snippet, "sentinel") {
		t.Fatalf("rows=%+v, want second indexed section from %s", rows, path)
	}
}
```

- [ ] **Step 3: Run Markdown tests and verify behavior**

Run: `go test ./cmd/backscroll -run 'TestMandatoryStartupIndexesMarkdown' -count=1`

Expected: PASS once startup policy invokes the existing Markdown registry before query handlers.

- [ ] **Step 4: Update machine-output tests**

For JSON, unmarshal stdout and assert no progress prose precedes the JSON. For robot mode, validate each result line and ensure a sync failure emits no cached rows:

```go
var jsonRows []minimalSearchResult
if err := json.Unmarshal(stdout.Bytes(), &jsonRows); err != nil {
	t.Fatalf("startup contaminated JSON stdout %q: %v", stdout.String(), err)
}
resultLine := regexp.MustCompile(`^result_[0-9]+_[a-z_]+=`)
for _, line := range strings.Split(strings.TrimSpace(robotStdout), "\n") {
	if strings.HasPrefix(line, "result_") && !resultLine.MatchString(line) {
		t.Fatalf("invalid robot line %q", line)
	}
}
if strings.Contains(failedSyncStdout, "result_") {
	t.Fatalf("sync failure emitted cached rows: %q", failedSyncStdout)
}
```

- [ ] **Step 5: Run focused output tests**

Run: `go test ./cmd/backscroll -run 'TestMandatoryStartupIndexesMarkdown|TestMachineModes|TestAutoSyncFailuresBlockCachedConsumers' -count=1`

Expected: PASS with progress/diagnostics routed according to output mode and no stale rows.

- [ ] **Step 6: Commit integration evidence**

```bash
git add cmd/backscroll/index_policy_test.go cmd/backscroll/diagnostics_test.go cmd/backscroll/markdown_registry_test.go
git commit -m "test(cli): prove database-backed markdown retrieval"
```

---

### Task 6: Convert Snapshot-Based Test Fixtures

**Files:**
- Modify: `cmd/backscroll/main_test.go`
- Modify: `cmd/backscroll/diagnostics_test.go`
- Modify: `cmd/backscroll/compat_diagnostics_test.go`
- Modify: `cmd/backscroll/legacy_sources_test.go`
- Modify: `cmd/backscroll/list_coverage_test.go`
- Modify: `cmd/backscroll/patterns_coverage_test.go`
- Modify: `cmd/backscroll/patterns_sequences_test.go`
- Modify: `cmd/backscroll/recover_test.go`
- Modify: `cmd/backscroll/index_policy_test.go`

**Interfaces:**
- Consumes: `setIndexPolicyEnv`, `writeInputManifest`, seeded SQLite helpers, and mandatory root startup.
- Produces: hermetic tests that never request stale snapshot behavior.

- [ ] **Step 1: Remove forbidden flag arguments from test invocations**

Run this guarded rewrite, then inspect the diff:

```bash
python3 - <<'PY'
from pathlib import Path
for path in Path('cmd/backscroll').glob('*_test.go'):
    text = path.read_text()
    text = text.replace(', "--indexed-only"', '')
    text = text.replace('"--indexed-only", ', '')
    text = text.replace('"--indexed-only"', '')
    path.write_text(text)
PY
gofmt -w cmd/backscroll/*_test.go
git diff -- cmd/backscroll
```

Delete obsolete tests whose sole requirement was accepting or bypassing with `--indexed-only`; do not merely rename them.

- [ ] **Step 2: Update direct helper signatures**

Change direct calls from:

```go
maybeAutoSync(cfg)
prepareIndex(context.Background(), cfg, indexDataRead, true)
```

into:

```go
maybeAutoSync(cfg, io.Discard)
prepareIndex(context.Background(), cfg, indexDataRead)
```

- [ ] **Step 3: Update legacy-source command tables**

Remove the `read` entry and use `[]string{"validate"}`. Retain `recover` to prove legacy configuration is still rejected before side effects when no recovery continuation is possible.

- [ ] **Step 4: Make seeded-index tests survive mandatory startup**

For each test that seeds SQLite directly, call `setIndexPolicyEnv(t, dbPath, t.TempDir())` and ensure its active input root is empty or contains the intended fixture. This preserves seeded rows while allowing the mandatory sync attempt to succeed.

- [ ] **Step 5: Run the command package**

Run: `go test ./cmd/backscroll -count=1`

Expected: PASS. Any failure mentioning machine-local inputs indicates a missing `HOME`/`BACKSCROLL_CONFIG_DIR` scrub; any missing seeded row indicates the fixture accidentally pointed startup sync at a destructive uuid-less source.

- [ ] **Step 6: Assert forbidden production/test registrations are gone**

Run: `rg -n --glob='*.go' -- '--indexed-only|newReadCmd|internal/reader' cmd internal`

Expected: no matches except quoted assertions that verify the forbidden flag/command is absent.

- [ ] **Step 7: Commit fixture migration**

```bash
git add cmd/backscroll/*_test.go
git commit -m "test(cli): migrate fixtures to mandatory sync"
```

---

### Task 7: Align Living Documentation and Shipped Skill

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/audit-integration.md`
- Modify: `docs/configuration.md`
- Modify: `docs/input-contract.md`
- Modify: `docs/patterns.md`
- Rewrite: `docs/read.md`
- Modify: `docs/sync.md`
- Modify: `.claude/skills/backscroll/SKILL.md`
- Modify: `.claude/skills/backscroll/ref-context-mode.md`
- Modify: `cmd/backscroll/skill_contract_test.go`

**Interfaces:**
- Consumes: final Cobra tree and ADR 0002.
- Produces: one living contract: active manifests -> mandatory startup sync -> perennial SQLite -> database-backed query.

- [ ] **Step 1: Update skill-contract assertions first**

Replace stale anchors with:

```go
anchors := []string{
	"Search discipline (hard rules)",
	"Drill the top hit",
	"artifact's vocabulary",
	"failed invocation is a syntax problem",
	"Two empty searches prove nothing",
	"Raw-file boundary",
	"--source-path",
	"mandatory startup sync",
	"backscroll validate",
}
```

Add assertions that the shipped skill contains neither `--indexed-only` nor an invocation beginning `backscroll read`.

- [ ] **Step 2: Run contract tests and verify they fail against stale docs**

Run: `go test ./cmd/backscroll -run 'TestBackscrollSkill|TestBackscrollLivingDocs' -count=1`

Expected: FAIL with stale `read`/`--indexed-only` references and invalid removed flags.

- [ ] **Step 3: Rewrite living guidance around the single data path**

Use this canonical wording where each document explains freshness:

```text
Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, list, patterns, status, and validate.
Use search --source-path for database-backed retrieval scoped to a known input path.
```

`docs/read.md` becomes a migration page explaining removal of direct read, with examples using `backscroll search --source-path "*session-id*" --all-projects --json`.

- [ ] **Step 4: Correct whole-document manifest examples**

Replace living examples of:

```toml
[inputs.decode]
format = "markdown"
```

with:

```toml
[inputs.decode]
format = "markdown_document"
```

Keep `markdown_sections` examples unchanged.

- [ ] **Step 5: Update repository structure documentation**

In `CLAUDE.md`, remove `cmd/backscroll/read.go`, remove `internal/reader`, remove `read`/`--indexed-only` from the command inventory, change the count from eleven to ten commands, and remove the `internal/reader` package-layout row. Add the mandatory root startup-sync decision to Key Design Decisions.

- [ ] **Step 6: Update the shipped skill and context reference**

Replace snapshot probes with ordinary mandatory-sync searches. Preserve the rule against raw `cat`, `jq`, Python, or filesystem session hunting, but state that database-backed `search --source-path` is the supported drill-down path. Replace `backscroll validate --indexed-only` and `backscroll status --indexed-only` with their flag-free forms.

- [ ] **Step 7: Run documentation contracts and stale-reference scan**

Run:

```bash
go test ./cmd/backscroll -run 'TestBackscrollSkill|TestBackscrollLivingDocs' -count=1
rg -n --glob='*.md' --glob='*.toml' -- '(backscroll read|--indexed-only|decode\.format = "markdown"|format = "markdown")' README.md CLAUDE.md docs/audit-integration.md docs/configuration.md docs/input-contract.md docs/patterns.md docs/read.md docs/sync.md .claude/skills/backscroll
```

Expected: contract tests PASS and the scan returns no living guidance matches. Historical roadmap/spec/plan records are intentionally outside this scan.

- [ ] **Step 8: Commit living documentation**

```bash
git add README.md CLAUDE.md docs/audit-integration.md docs/configuration.md docs/input-contract.md docs/patterns.md docs/read.md docs/sync.md .claude/skills/backscroll/SKILL.md .claude/skills/backscroll/ref-context-mode.md cmd/backscroll/skill_contract_test.go
git commit -m "docs: require manifest sync and SQLite retrieval"
```

---

### Task 8: Run Full Verification and Update the PR

**Files:**
- Verify: all modified files
- Existing ADR: `docs/adr/0002-exigir-sync-y-consulta-desde-sqlite.md`

**Interfaces:**
- Consumes: Tasks 1-7.
- Produces: formatted, tested, coverage-compliant implementation pushed to PR #45.

- [ ] **Step 1: Format and inspect repository state**

Run:

```bash
gofmt -w cmd/backscroll internal
git diff --check
git status --short
```

Expected: no formatting errors or whitespace diagnostics; only intended files are modified.

- [ ] **Step 2: Run static checks**

Run: `just check`

Expected: PASS (`gofmt --check` and `go vet`).

- [ ] **Step 3: Run the complete test suite**

Run: `just test`

Expected: PASS across all packages.

- [ ] **Step 4: Run the release-equivalent CI gate**

Run: `just ci`

Expected: build succeeds, scrubbed-HOME tests pass, and aggregate statement coverage is >=85%.

- [ ] **Step 5: Validate ADR records when Rootline is available**

```bash
if command -v rootline >/dev/null 2>&1; then
  rootline validate docs/adr/0001-declarar-frontera-documentacion-cli.md docs/adr/0002-exigir-sync-y-consulta-desde-sqlite.md --strict --output table
fi
```

Expected: both ADRs validate successfully.

- [ ] **Step 6: Review the final diff against issue #44**

Run:

```bash
git diff origin/main...HEAD --stat
git log --oneline origin/main..HEAD
gh issue view 44 --repo pablontiv/backscroll --json body --jq .body
```

Check every acceptance checkbox against a test, code path, or living-doc update before claiming completion.

- [ ] **Step 7: Push the implementation commits to PR #45**

```bash
git push origin docs/issue-44-db-only-sync-design
gh pr view 45 --repo pablontiv/backscroll --json url,headRefName,statusCheckRollup
```

Expected: PR #45 points at the implementation branch and CI starts or reports success.
