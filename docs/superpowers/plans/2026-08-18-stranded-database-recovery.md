# Stranded Database Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `backscroll recover --from <stranded.db> [--dry-run]` to replace the configured active database with the verified union of its active records and one stranded historical database without mutating either input during planning.

**Architecture:** Extend Plan 1’s schema-only compatibility boundary with the recovery types and canonical record representation needed by this consumer, then build one complete active-first union plan. A separate recovery consumer creates and verifies a fresh current-schema sibling database, preserves the original active database, and performs same-filesystem atomic replacement. Only after `recover` is registered and functional does this plan activate one shared fail-closed index policy and user-facing executable continuations.

**Tech Stack:** Go 1.26.2, `database/sql`, `modernc.org/sqlite`, `google/uuid`, Cobra 1.10.2, SHA-256, filesystem sync/rename primitives, table-driven Go integration tests, Just

**Spec:** `docs/superpowers/specs/2026-08-18-systemic-index-compatibility-design.md`

## Global Constraints

- This is delivery Plan 3 but executes second: Plan 1 inspection/migration primitives → Plan 3 recovery and blocking-policy activation → Plan 2 manifest ingestion → Plan 4 shipped guidance validation.
- This plan depends only on Plan 1’s `compat.InspectIndex`, `compat.SchemaShape`, schema diagnostics, and current-shape verification. Plan 3 itself introduces recovery types and the shared canonical `models.IndexedRecord` only when recovery consumes them.
- The only command is `backscroll recover --from <stranded.db> [--dry-run]`; destination is the configured active database.
- Do not add `--into`, `--force`, `--skip`, `--skip-conflicts`, `--merge`, `--partial`, multiple `--from` values, cross-machine transfer, synchronization, replication, or conflict resolution.
- Resolve active and stranded paths before opening. If both resolve to the same path, account for one logical input once while still using fresh destination and backup on apply.
- Open every distinct input with SQLite `mode=ro`; never migrate, vacuum, journal-mode change, or otherwise mutate either planning input. Stranded bytes and sidecar metadata remain immutable on dry-run, success, and failure.
- Identity is valid UUID first; only an empty UUID falls back to exact stored `(source_path, ordinal)` with non-negative ordinal. Payload hash proves equivalence only and never creates identity.
- Collapse only same-identity/equivalent-payload duplicates. Same identity/different payload or any uninterpretable row aborts the entire plan and apply.
- Account for every row from both inputs as importable, exact duplicate, conflicting, or uninterpretable before any write.
- Dry-run and apply call the identical planner; dry-run creates no destination, temporary database, backup, journal, or other file.
- Apply creates a fresh current-schema temporary database beside active, imports and verifies in one transaction, closes handles, independently reopens/verifies, preserves active as a unique backup, atomically replaces on the same filesystem, and fsyncs the directory after backup and replacement.
- A failed recovery never reports success and leaves the original active path intact or a restorable backup with an explicit diagnostic. Never delete the backup automatically.
- Stale-index refusal, cached-fallback removal, indexed-consumer changes, diagnostic command behavior, direct-read exemption tests, and executable-continuation tests activate only after `recover` is registered and functional in this plan.
- Every user-facing blocking diagnostic has non-empty argv that resolves and executes through the real Cobra tree; no test is skipped or deferred at any plan boundary.
- `backscroll read` remains behaviorally unchanged and bypasses index policy because it reads the supplied file directly; test that behavior without modifying or staging `cmd/backscroll/read.go`.
- Tests set HOME, BACKSCROLL_CONFIG_DIR, and BACKSCROLL_DATABASE_PATH to temporary locations and compare input bytes, mtimes, and sidecar inventories.
- Follow strict RED → GREEN → TRIANGULATE → REFACTOR. Run focused tests before `just check`, `just test`, and `just ci`.
- Commit commands below are future implementation instructions only. Do not stage or commit during planning.

---

## File map

| Path | Responsibility |
|---|---|
| `internal/models/indexed_record.go` | Canonical indexed record shared by storage queries and recovery without an import cycle. |
| `internal/storage/records.go` | Keep query ownership in storage while returning `models.IndexedRecord`. |
| `internal/compat/types.go` | Add only recovery types now consumed by this plan. |
| `internal/compat/recovery.go` | Compute identities and payload hashes and return a complete union plan or typed diagnostics. |
| `internal/compat/recovery_test.go` | Identity/equivalence/conflict/uninterpretable matrix and accounting. |
| `internal/storage/recovery_records.go` | Read each historical lineage into current canonical `models.IndexedRecord` without writing inputs. |
| `internal/storage/recovery_destination.go` | Create fresh destination, import one transaction, database-level verify, and independent reopen verify. |
| `internal/storage/recovery_destination_test.go` | Current-schema, count, FK, FTS, shape, queryability, and rollback tests. |
| `internal/recovery/recovery.go` | Resolve/dedupe paths, dry-run/apply orchestration, backup, atomic replacement, fsync, and report. |
| `internal/recovery/recovery_test.go` | No-write dry-run, same-path, immutable stranded source, backup, replacement, and failure invariants. |
| `cmd/backscroll/recover.go` | Cobra flags, config resolution, output formatting, and exit behavior. |
| `cmd/backscroll/recover_test.go` | Real CLI contract and report fields. |
| `cmd/backscroll/main.go` | Register `newRecoverCmd`. |
| `cmd/backscroll/index_policy.go` | Shared command classes, fresh inspection, stale refusal, and diagnostic rendering. |
| `cmd/backscroll/index_policy_test.go` | Complete indexed-consumer, output-mode, and cached-fallback matrix. |
| `cmd/backscroll/compat_diagnostics_test.go` | Read-only diagnostics, direct-read exemption, and executable continuation coverage. |
| `cmd/backscroll/{search,list,patterns,rebuild,purge,annotate,status,validate}.go` | Delegate index health and preparation to the shared policy. |
| `cmd/backscroll/sync_helpers.go` | Propagate every discovery/decode/sync failure instead of falling back to cached data. |
| `scripts/calibration-extract/main.go` | Inspect before direct indexed-record extraction. |
| `tests/fixtures/recovery/*.sql` | Active/stranded lineage and conflict fixtures copied into temporary databases by tests. |

### Task 1: Introduce the recovery contract and shared canonical indexed record

**Files:**
- Create: `internal/models/indexed_record.go`
- Modify: `internal/storage/records.go:9-107`
- Modify: `internal/storage/unit_test.go:525-627,1782-1814,2877-2908,3064-3066`
- Modify: `internal/compat/types.go`

**Interfaces:**
- Consumes: Plan 1 `compat.SchemaShape`, `compat.Diagnostic`, and existing `storage.IndexedRecord` fields.
- Produces: `models.IndexedRecord`, `compat.RecoveryInput`, `compat.CanonicalRecord`, and `compat.RecoveryPlan`. `storage.IndexedRecordQuery` remains storage-owned; `func (d *Database) QueryIndexedRecords(q IndexedRecordQuery) ([]models.IndexedRecord, error)` is the only query signature change.

- [ ] **Step 1: Write the compile-time ownership test first**

Add `TestQueryIndexedRecordsReturnsCanonicalModel` in `internal/storage/unit_test.go`. Assign the result of `QueryIndexedRecords` to `var got []models.IndexedRecord` and assert all existing source/path/ordinal/role/text/project/UUID/timestamp/content-type sentinels survive.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/storage -run '^TestQueryIndexedRecordsReturnsCanonicalModel$'`

Expected: FAIL because `models.IndexedRecord` does not exist and `QueryIndexedRecords` returns the storage-local type.

- [ ] **Step 3: Move only the canonical record and add recovery-owned types**

```go
// internal/models/indexed_record.go
type IndexedRecord struct {
    Source      string
    SourcePath  string
    Ordinal     int64
    Role        string
    Text        string
    Project     *string
    UUID        *string
    Timestamp   *string
    ContentType string
}

// internal/compat/types.go
const (
    CodeRecoveryConflict   Code = "recovery_conflict"
    CodeUninterpretableRow Code = "uninterpretable_row"
)

type RecoveryInput struct {
    Shape    SchemaShape
    Records  []models.IndexedRecord
    RowCount int
}

type CanonicalRecord struct {
    Record      models.IndexedRecord
    PayloadHash string
}

type RecoveryPlan struct {
    InputShapes     []SchemaShape
    Records         []CanonicalRecord
    ExactDuplicates int
}
```

Delete only the duplicate `storage.IndexedRecord` definition, import `internal/models` in `records.go`, and make scan locals plus the return type use `models.IndexedRecord`. Keep `IndexedRecordQuery` and SQL query construction in storage. Do not introduce manifest types or move unrelated storage models.

- [ ] **Step 4: GREEN and triangulate existing query filters**

Run: `go test ./internal/storage -run '^(TestQueryIndexedRecordsReturnsCanonicalModel|TestQueryIndexedRecords|TestQueryIndexedRecordsLimitAndOffset|TestQueryIndexedRecordsComplexFilters)$'`

Expected: PASS; ownership changes without changing query behavior.

- [ ] **Step 5: Run affected packages and commit the coherent ownership move**

Run: `go test ./internal/models ./internal/storage ./internal/compat`

Expected: PASS.

```bash
git add internal/models/indexed_record.go internal/storage/records.go internal/storage/unit_test.go internal/compat/types.go
git commit -m "refactor(recovery): share canonical indexed record model"
```

### Task 2: Adapt every supported input row into the current canonical record shape

**Files:**
- Create: `internal/storage/recovery_records.go`
- Create: `internal/storage/recovery_records_test.go`
- Create: `tests/fixtures/recovery/active-v13.sql`
- Create: `tests/fixtures/recovery/stranded-v3-no-source-metadata.sql`
- Create: `tests/fixtures/recovery/stranded-v7.sql`

**Interfaces:**
- Consumes: Plan 1 `compat.InspectIndex`, Plan 1 `compat.SchemaShape`, and Task 1 `models.IndexedRecord`.
- Produces: Task 1 `compat.RecoveryInput` with fields `Shape compat.SchemaShape`, `Records []models.IndexedRecord`, and `RowCount int`; plus `func ReadRecoveryInput(ctx context.Context, db *Database) (compat.RecoveryInput, *compat.Diagnostic, error)`.

- [ ] **Step 1: Write historical adaptation tests first**

```go
func TestReadRecoveryInputAdaptsSupportedLineages(t *testing.T) {
    for _, fixture := range []string{"active-v13.sql", "stranded-v3-no-source-metadata.sql", "stranded-v7.sql"} {
        t.Run(fixture, func(t *testing.T) {
            db := openRecoveryFixtureReadOnly(t, fixture)
            got, diag, err := ReadRecoveryInput(context.Background(), db)
            if err != nil || diag != nil { t.Fatalf("err=%v diagnostic=%+v", err, diag) }
            if got.RowCount == 0 || got.RowCount != len(got.Records) { t.Fatalf("input=%+v", got) }
        })
    }
}
```

Seed source, path, ordinal, role, text, project, UUID, timestamp, and content type sentinels and assert exact preservation.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/storage -run '^TestReadRecoveryInput'`

Expected: FAIL with `undefined: ReadRecoveryInput`.

- [ ] **Step 3: Implement read-only adapters keyed by inspected shape**

Use a switch over Plan 1’s stable lineage/signature identifier, not migration number alone. Each adapter runs SELECT-only queries and maps absent historical columns to the same canonical defaults current storage migration would produce. Unknown/readable shape returns `CodeUnsupportedLineage`; a row missing required canonical payload returns `CodeUninterpretableRow`. These internal diagnostics do not print command text or set `Continuation`; Task 8 renders them only after `recover` is functional.

```go
func ReadRecoveryInput(ctx context.Context, db *Database) (compat.RecoveryInput, *compat.Diagnostic, error) {
    plan, diag, err := compat.InspectIndex(ctx, db.DB())
    if err != nil || diag != nil { return compat.RecoveryInput{}, diag, err }
    shape := plan.From
    records, err := readRecordsForSignature(ctx, db.DB(), shape.Signature)
    if err != nil { return compat.RecoveryInput{}, nil, err }
    return compat.RecoveryInput{Shape: shape, Records: records, RowCount: len(records)}, nil, nil
}
```

- [ ] **Step 4: GREEN and triangulate unknown/corrupt rows**

Run: `go test ./internal/storage -run '^(TestReadRecoveryInputAdaptsSupportedLineages|TestReadRecoveryInputRejectsUnknownShape|TestReadRecoveryInputRejectsMissingCanonicalPayload|TestReadRecoveryInputPerformsNoWrites)$'`

Expected: PASS; SQLite `PRAGMA query_only` remains true and fixture bytes do not change.

- [ ] **Step 5: Refactor adapter column lists and run storage tests**

Run: `go test ./internal/storage`

Expected: PASS.

- [ ] **Step 6: Commit the canonical read adapters**

```bash
git add internal/storage/recovery_records.go internal/storage/recovery_records_test.go tests/fixtures/recovery
git commit -m "feat(recovery): adapt supported indexes read-only"
```

### Task 3: Build the complete active-plus-stranded union plan

**Files:**
- Create: `internal/compat/recovery.go`
- Create: `internal/compat/recovery_test.go`

**Interfaces:**
- Consumes: `compat.RecoveryInput`, `compat.RecoveryPlan`, `compat.CanonicalRecord`, and `compat.Diagnostic` introduced by Task 1 and populated by Task 2.
- Produces: `func PlanRecovery(inputs []RecoveryInput) (RecoveryPlan, []Diagnostic, error)` and private exact identity type `type recordIdentity struct { Kind string; UUID string; SourcePath string; Ordinal int64 }`. This keeps `internal/compat` independent of `internal/storage` and avoids an import cycle.

- [ ] **Step 1: Write the full identity/conflict matrix**

Add `TestRecoverIdentityAndConflictMatrixAcrossInputs` with cases: valid UUID beats differing path/ordinal; empty UUID falls back to exact path/ordinal; equal identity/equal canonical payload collapses; equal identity/different payload conflicts; equal content under different identities remains two records; invalid non-empty UUID is uninterpretable; empty UUID plus empty path or negative ordinal is uninterpretable. Run each case both within one input and across active/stranded inputs.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/compat -run '^TestRecoverIdentityAndConflictMatrixAcrossInputs$'`

Expected: FAIL with `undefined: PlanRecovery`.

- [ ] **Step 3: Implement identity and canonical payload hashing**

Validate UUID using `uuid.Parse`; do not invent one. Serialize all canonical payload fields except identity in a fixed length-prefixed order and SHA-256 the bytes. Preserve exact stored `source_path`; do not normalize case, separators, symlinks, host, or project roots.

```go
func identityOf(r models.IndexedRecord) (recordIdentity, error) {
    if r.UUID != nil && *r.UUID != "" {
        if _, err := uuid.Parse(*r.UUID); err != nil { return recordIdentity{}, err }
        return recordIdentity{Kind: "uuid", UUID: *r.UUID}, nil
    }
    if r.SourcePath == "" || r.Ordinal < 0 { return recordIdentity{}, errUnsafeIdentity }
    return recordIdentity{Kind: "path_ordinal", SourcePath: r.SourcePath, Ordinal: r.Ordinal}, nil
}
```

Sort output deterministically by identity kind, UUID, source path, ordinal, then payload hash. Set `InputShapes` active first and `ExactDuplicates` to the number collapsed. Return no applicable records when any diagnostic exists.

- [ ] **Step 4: GREEN and triangulate complete accounting**

Run: `go test ./internal/compat -run '^(TestRecoverIdentityAndConflictMatrixAcrossInputs|TestPlanRecoveryAccountsForEveryInputRow|TestPlanRecoveryPreservesDistinctHashEquivalentIdentities|TestPlanRecoveryIsDeterministic)$'`

Expected: PASS; `sum(input rows) == len(plan.Records) + plan.ExactDuplicates` only when diagnostics are empty, while conflict/uninterpretable cases name every rejected identity.

- [ ] **Step 5: Refactor hashing and run compat tests**

Run: `go test ./internal/compat`

Expected: PASS.

- [ ] **Step 6: Commit the stateless union planner**

```bash
git add internal/compat/recovery.go internal/compat/recovery_test.go
git commit -m "feat(recovery): plan deterministic active stranded union"
```

### Task 4: Add the exact Cobra command, path resolution, same-path deduplication, and dry-run

**Files:**
- Create: `internal/recovery/recovery.go`
- Create: `internal/recovery/recovery_test.go`
- Create: `cmd/backscroll/recover.go`
- Create: `cmd/backscroll/recover_test.go`
- Modify: `cmd/backscroll/main.go:59-83`

**Interfaces:**
- Consumes: `config.Config.DatabasePath`, `storage.OpenReadOnly`, `storage.ReadRecoveryInput`, `compat.PlanRecovery`.
- Produces: `type Options struct { ActivePath string; FromPath string; DryRun bool }`, `type Report struct { ActivePath string; BackupPath string; InputCounts []int; ExactDuplicates int; FinalCount int; Shapes []compat.SchemaShape; Conflicts []compat.Diagnostic }`, `func Execute(ctx context.Context, opts Options) (Report, error)`, and `func newRecoverCmd(stdout, stderr io.Writer) *cobra.Command`.

- [ ] **Step 1: Write CLI and no-write dry-run tests**

Add `TestRecoverDryRunMatchesUnionApplyPlanWithoutWrites` and `TestRecoverSameResolvedPathIsOneInput`. Before dry-run, inventory every file in the active directory with bytes and mtimes. Assert identical inventory afterward, `InputCounts` has one entry for same-path resolution, and stdout contains active path, per-input counts, duplicate count, final count, shapes, and intended replacement path.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/recovery ./cmd/backscroll -run '^(TestRecoverDryRunMatchesUnionApplyPlanWithoutWrites|TestRecoverSameResolvedPathIsOneInput)$'`

Expected: FAIL because packages/command do not exist and `buildRootCmd` has no `recover`.

- [ ] **Step 3: Implement exact command and read-only planner path**

`newRecoverCmd` uses `Use: "recover"`, `cobra.NoArgs`, required string flag `--from`, and bool `--dry-run`; it defines no other recovery flags. Resolve both paths with `filepath.Abs`, `filepath.EvalSymlinks` when possible, and `os.SameFile` after stat. Open distinct inputs via `storage.OpenReadOnly` only. Pass active first, stranded second; pass one input when paths resolve identically.

```go
cmd.Flags().StringVar(&from, "from", "", "path to one stranded Backscroll database")
cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan and report recovery without writing files")
_ = cmd.MarkFlagRequired("from")
```

For dry-run, return immediately after planning/report construction and register `newRecoverCmd` in `buildRootCmd`. Do not activate shared index blocking or executable-continuation tests yet: apply becomes fully functional in Tasks 5–6, then Tasks 7–8 activate and prove the policy with no skip.

- [ ] **Step 4: GREEN and triangulate forbidden command shapes**

Run: `go test ./internal/recovery ./cmd/backscroll -run '^(TestRecoverDryRunMatchesUnionApplyPlanWithoutWrites|TestRecoverSameResolvedPathIsOneInput|TestRecoverRejectsMissingFrom|TestRecoverHasNoGeneralMergeFlags)$'`

Expected: PASS; `--into`, repeated `--from`, `--force`, and `--partial` are rejected by Cobra.

- [ ] **Step 5: Refactor report formatting and run command packages**

Run: `go test ./internal/recovery ./cmd/backscroll`

Expected: PASS.

- [ ] **Step 6: Commit command and dry-run boundary**

```bash
git add internal/recovery cmd/backscroll/recover.go cmd/backscroll/recover_test.go cmd/backscroll/main.go
git commit -m "feat(cli): add read-only stranded recovery dry run"
```

### Task 5: Import the verified union into one fresh current-schema transaction

**Files:**
- Create: `internal/storage/recovery_destination.go`
- Create: `internal/storage/recovery_destination_test.go`
- Modify: `internal/recovery/recovery.go`

**Interfaces:**
- Consumes: applicable `compat.RecoveryPlan` and existing current-schema `storage.Open`/`SyncFiles` primitives.
- Produces: `func CreateRecoveryDestination(ctx context.Context, dir string, plan compat.RecoveryPlan) (path string, err error)` and `func VerifyRecoveryDestination(ctx context.Context, path string, plan compat.RecoveryPlan) error`.

- [ ] **Step 1: Write fresh-destination and rollback tests**

Add `TestRecoverUnionPreservesActiveAndStrandedRecords` and `TestRecoverConflictOrUninterpretableRollsBackEverything`. Seed unique active/stranded records plus one exact duplicate; assert fresh destination contains both unique sets once. Inject an import failure and assert no committed destination and unchanged inputs.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/storage ./internal/recovery -run '^(TestRecoverUnionPreservesActiveAndStrandedRecords|TestRecoverConflictOrUninterpretableRollsBackEverything)$'`

Expected: FAIL with `undefined: CreateRecoveryDestination`.

- [ ] **Step 3: Implement one-transaction destination creation and verification**

Create the temp file with `os.CreateTemp(dir, ".backscroll-recover-*.db")`, close it, initialize current schema, begin one SQL transaction, insert the canonical union through transaction-aware storage helpers, verify source accounting, union row count, unique identities, `PRAGMA foreign_key_check`, FTS row/count consistency, current schema signature, and representative exact queryability, then commit once.

Close the destination and independently reopen with `OpenReadOnly`; `VerifyRecoveryDestination` reruns row count, identity, FTS, shape, and representative query checks against committed bytes. Return the path only after independent verification.

- [ ] **Step 4: GREEN and triangulate post-commit independent failure**

Run: `go test ./internal/storage ./internal/recovery -run '^(TestRecoverUnionPreservesActiveAndStrandedRecords|TestRecoverConflictOrUninterpretableRollsBackEverything|TestRecoveryDestinationStartsFreshAtCurrentSchema|TestRecoveryDestinationIndependentVerificationRejectsTamper)$'`

Expected: PASS; tampering between close and independent verification prevents replacement.

- [ ] **Step 5: Refactor transaction helpers and run affected packages**

Run: `go test ./internal/storage ./internal/recovery`

Expected: PASS.

- [ ] **Step 6: Commit fresh destination apply logic**

```bash
git add internal/storage/recovery_destination.go internal/storage/recovery_destination_test.go internal/recovery/recovery.go
git commit -m "feat(recovery): build verified union destination"
```

### Task 6: Preserve active backup and atomically replace on the same filesystem

**Files:**
- Modify: `internal/recovery/recovery.go`
- Modify: `internal/recovery/recovery_test.go`
- Modify: `cmd/backscroll/recover_test.go`

**Interfaces:**
- Consumes: independently verified sibling destination from Task 5.
- Produces: `func replaceActiveWithBackup(activePath, verifiedTempPath string) (backupPath string, err error)` and complete success report.

- [ ] **Step 1: Write replacement and immutability tests**

Add `TestRecoverAtomicallyReplacesAndPreservesActiveBackup` and `TestRecoverStrandedSourceIsReadOnly`. Capture active and stranded bytes, mtimes, inode/file IDs where available, and `-wal`/`-shm` sidecar inventories across dry-run, success, conflict failure, and injected replacement failure. Assert backup bytes equal original active bytes and stranded metadata never changes.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/recovery ./cmd/backscroll -run '^(TestRecoverAtomicallyReplacesAndPreservesActiveBackup|TestRecoverStrandedSourceIsReadOnly)$'`

Expected: FAIL because apply does not yet back up or replace active.

- [ ] **Step 3: Implement backup, fsync, and replacement**

Close every input and destination handle first. Choose a unique sibling backup name `.backscroll.db.backup-<UTC timestamp>-<six random hex>` without overwriting. Rename active to backup, fsync the directory, rename verified temp to active, fsync again. If the second rename fails, rename backup back to active and fsync; return an explicit error whether restoration succeeds or leaves a restorable backup.

```go
func syncDir(path string) error {
    f, err := os.Open(path)
    if err != nil { return err }
    defer f.Close()
    return f.Sync()
}
```

Assert `filepath.Dir(activePath) == filepath.Dir(verifiedTempPath)` before either rename; reject cross-filesystem paths rather than copying.

- [ ] **Step 4: GREEN and triangulate replacement failure**

Run: `go test ./internal/recovery ./cmd/backscroll -run '^(TestRecoverAtomicallyReplacesAndPreservesActiveBackup|TestRecoverStrandedSourceIsReadOnly|TestRecoverReplacementFailureRestoresActive|TestRecoverNeverDeletesBackup)$'`

Expected: PASS; success reports active/backup paths and counts, while failure retains a valid active path or names the exact restorable backup.

- [ ] **Step 5: Refactor platform-specific rename injection and run packages**

Run: `go test ./internal/recovery ./cmd/backscroll`

Expected: PASS.

- [ ] **Step 6: Commit atomic replacement as one rollback boundary**

```bash
git add internal/recovery/recovery.go internal/recovery/recovery_test.go cmd/backscroll/recover_test.go
git commit -m "feat(recovery): atomically replace active with preserved backup"
```

### Task 7: Activate one fail-closed policy across every indexed consumer

**Files:**
- Create: `cmd/backscroll/index_policy.go`
- Create: `cmd/backscroll/index_policy_test.go`
- Modify: `cmd/backscroll/search.go:99-145`
- Modify: `cmd/backscroll/list.go:62-88`
- Modify: `cmd/backscroll/patterns.go:87-137`
- Modify: `cmd/backscroll/rebuild.go:31-109`
- Modify: `cmd/backscroll/purge.go:35-50`
- Modify: `cmd/backscroll/annotate.go:48-69`
- Modify: `cmd/backscroll/sync_helpers.go:16-160`
- Modify: `scripts/calibration-extract/main.go:39-49`

**Interfaces:**
- Consumes: Plan 1 `storage.OpenCompatible`, Task 6’s functional `recover`, existing `maybeAutoSync`, and typed `compat.Diagnostic` values.
- Produces: `type indexCommandClass uint8`, `func prepareIndex(ctx context.Context, cfg *config.Config, class indexCommandClass, autoSync bool) (*storage.Database, *compat.Diagnostic, error)`, `func continuationFor(d compat.Diagnostic, activePath string) compat.Diagnostic`, and `func writeDiagnostic(stdout, stderr io.Writer, d compat.Diagnostic, jsonMode bool) error`.

- [ ] **Step 1: Add the complete stale-consumer failure matrix**

```go
func TestStaleIndexBlocksIndexBackedCommands(t *testing.T) {
    cases := []struct{ name string; argv []string; mutation bool }{
        {"search", []string{"search", "sentinel"}, false},
        {"search-indexed-only", []string{"search", "sentinel", "--indexed-only"}, false},
        {"search-json", []string{"search", "sentinel", "--json"}, false},
        {"search-robot", []string{"search", "sentinel", "--robot"}, false},
        {"list", []string{"list"}, false},
        {"list-json", []string{"list", "--json"}, false},
        {"patterns", []string{"patterns", "--kind", "commands"}, false},
        {"rebuild", []string{"rebuild"}, true},
        {"purge", []string{"purge", "--before", "2030-01-01"}, true},
        {"annotate", []string{"annotate", "--uuid", "u", "--kind", "correction", "--label", "x"}, true},
    }
    // Seed cached sentinel output, force migration or sync failure, execute argv,
    // assert non-zero, no sentinel output, and byte-identical DB for mutations.
}
```

- [ ] **Step 2: Run RED and observe the cached fallback**

Run: `go test ./cmd/backscroll -run '^TestStaleIndexBlocksIndexBackedCommands$'`

Expected: FAIL because indexed reads warn and continue with cached rows and mutations open the database outside one shared policy.

- [ ] **Step 3: Implement fresh preparation and remove every cached fallback**

Define `indexDataRead`, `indexMutation`, `indexDiagnostic`, and `indexRemediation`. `prepareIndex` performs fresh inspection on every invocation, applies Plan 1 migration only for data/mutation preparation, and returns before any query, mutation, success output, or zero-result claim on inspection, migration, discovery, decode, or sync failure. It never caches diagnostics.

`continuationFor` receives the resolved active path and sets exact argv `[]string{"recover", "--from", activePath, "--dry-run"}` only now that `recover` exists. It rejects an empty path and never writes a literal placeholder. Replace direct opens in the listed indexed commands. Make `maybeAutoSync` return unknown-reader, discovery, hash, parse, and `SyncFiles` errors instead of logging and continuing. Plan 2 later adds all-manifest preflight but consumes this already-active fail-closed boundary. The calibration extractor calls `compat.InspectIndex` before `QueryIndexedRecords` and aborts on every diagnostic.

- [ ] **Step 4: GREEN and triangulate output-mode bypasses**

Run: `go test ./cmd/backscroll ./scripts/calibration-extract -run '^(TestStaleIndexBlocksIndexBackedCommands|TestIndexedOnlyDoesNotBypassStaleBlock|TestMachineModesCarryDiagnosticCodeAndContinuation|TestCalibrationExtractRejectsUnsupportedIndex)$'`

Expected: PASS; indexed-only, JSON, robot, source filters, and empty-result paths cannot bypass refusal or emit cached data.

- [ ] **Step 5: Refactor adapters and run affected packages**

Run: `go test ./cmd/backscroll ./scripts/calibration-extract`

Expected: PASS with no skipped tests.

- [ ] **Step 6: Commit the activation as one rollback unit**

```bash
git add cmd/backscroll/index_policy.go cmd/backscroll/index_policy_test.go cmd/backscroll/search.go cmd/backscroll/list.go cmd/backscroll/patterns.go cmd/backscroll/rebuild.go cmd/backscroll/purge.go cmd/backscroll/annotate.go cmd/backscroll/sync_helpers.go scripts/calibration-extract/main.go
git commit -m "fix(cli): block every stale index consumer"
```

### Task 8: Make diagnostics read-only and prove every continuation and direct-read exemption

**Files:**
- Modify: `cmd/backscroll/status.go:44-147`
- Modify: `cmd/backscroll/validate.go:37-69`
- Modify: `cmd/backscroll/main_test.go:20-95`
- Create: `cmd/backscroll/compat_diagnostics_test.go`
- Test unchanged: `cmd/backscroll/read.go:12-113`

**Interfaces:**
- Consumes: `prepareIndex(..., indexDiagnostic, false)`, `continuationFor`, `writeDiagnostic`, and the registered `newRecoverCmd`.
- Produces: read-only unhealthy `status`/`validate` behavior plus `TestDirectReadRemainsAvailableButClaimsNoIndexFreshness` and `TestBlockingDiagnosticsHaveExecutableContinuations`.

- [ ] **Step 1: Write the diagnostic and direct-read tests**

`TestDirectReadRemainsAvailableButClaimsNoIndexFreshness` creates an unsupported database plus a direct JSONL fixture, executes the existing `read`, and asserts decoded file content appears while `usable`, `fresh`, and `current index` claims do not. It also asserts the unsupported database bytes remain unchanged.

`TestBlockingDiagnosticsHaveExecutableContinuations` builds each real unhealthy scenario rather than enumerating hypothetical codes: unsupported lineage, migration failure, stale sync, recovery conflict, and uninterpretable row. For each emitted diagnostic it requires non-empty argv, builds a fresh `buildRootCmd`, sets the exact argv, executes Cobra, and accepts a domain remediation error only after command/flag resolution. Every subcase must execute and pass at this task boundary.

- [ ] **Step 2: Run RED**

Run: `go test ./cmd/backscroll -run '^(TestDirectReadRemainsAvailableButClaimsNoIndexFreshness|TestBlockingDiagnosticsHaveExecutableContinuations)$'`

Expected: FAIL because `status`/`validate` still mutate or continue and diagnostic continuation rendering is not yet centralized. The failure must not be an unresolved `recover` command because Task 4 already registered it.

- [ ] **Step 3: Make diagnostic commands read-only without touching direct read**

Remove auto-sync and write-mode `storage.Open` from `status` and `validate`. Both inspect read-only, print the primary typed diagnostic plus independently safe secondary diagnostics, emit the same `code` and `continuation` fields in JSON, and exit non-zero when unhealthy. Keep `cmd/backscroll/read.go` byte-for-byte unchanged: its existing `newReadCmd`, `runRead`, and `runReadSemantic` continue to avoid `prepareIndex`.

Update only the shared `testEnv` helper in `main_test.go` to use `t.Setenv` and always set HOME, BACKSCROLL_CONFIG_DIR, and BACKSCROLL_DATABASE_PATH under temporary directories.

- [ ] **Step 4: GREEN and triangulate no-write diagnostics**

Run: `go test ./cmd/backscroll -run '^(TestDirectReadRemainsAvailableButClaimsNoIndexFreshness|TestStatusUnhealthyIsReadOnly|TestValidateUnhealthyIsReadOnly|TestBlockingDiagnosticsHaveExecutableContinuations)$'`

Expected: PASS with no skipped tests; no status/validate invocation changes database bytes or sidecars, and every printed continuation resolves through Cobra.

- [ ] **Step 5: Run the CLI package**

Run: `go test ./cmd/backscroll`

Expected: PASS with no real user config access.

- [ ] **Step 6: Commit diagnostics and proof without staging `read.go`**

```bash
git add cmd/backscroll/status.go cmd/backscroll/validate.go cmd/backscroll/main_test.go cmd/backscroll/compat_diagnostics_test.go
git commit -m "fix(cli): expose safe recovery from incompatible indexes"
```

### Task 9: Close issue #32 and prove the activated compatibility boundary

**Files:**
- Modify: `internal/compat/recovery_test.go`
- Modify: `internal/storage/recovery_destination_test.go`
- Modify: `internal/recovery/recovery_test.go`
- Modify: `cmd/backscroll/recover_test.go`
- Modify: `cmd/backscroll/index_policy_test.go`
- Modify: `cmd/backscroll/compat_diagnostics_test.go`

**Interfaces:**
- Consumes: all Plan 3 behavior plus Plan 1’s migration evidence.
- Produces: complete named #32 closure evidence, active migration/blocking/direct-read/continuation evidence for the migration-and-operability portion of #31, and final command contracts for Plans 2 and 4.

- [ ] **Step 1: Run all named #32 tests together**

Run: `go test ./internal/compat ./internal/storage ./internal/recovery ./cmd/backscroll -run '^(TestRecoverDryRunMatchesUnionApplyPlanWithoutWrites|TestRecoverUnionPreservesActiveAndStrandedRecords|TestRecoverIdentityAndConflictMatrixAcrossInputs|TestRecoverConflictOrUninterpretableRollsBackEverything|TestRecoverSameResolvedPathIsOneInput|TestRecoverAtomicallyReplacesAndPreservesActiveBackup|TestRecoverStrandedSourceIsReadOnly)$'`

Expected: PASS with no skipped case.

- [ ] **Step 2: Run the activated migration and command-policy boundary**

Run: `go test ./internal/compat ./internal/storage ./cmd/backscroll -run '^(TestCheckedInReleaseSchemaManifestIsComplete|TestPublishedGoLineagesUpgradeLosslessly|TestHistoricalLineageWithoutSourceMetadataUpgradesLosslessly|TestMigrationSnapshotAndRollbackOnDestructiveFailure|TestStaleIndexBlocksIndexBackedCommands|TestDirectReadRemainsAvailableButClaimsNoIndexFreshness|TestBlockingDiagnosticsHaveExecutableContinuations)$'`

Expected: PASS with no skipped test and no cached fallback. This proves the migration and operability portions of #31 but does not close #31; Plan 2 must still supply manifest-only ingestion evidence.

- [ ] **Step 3: Run repository gates**

Run: `just check`

Expected: PASS.

Run: `just test`

Expected: PASS.

Run: `just ci`

Expected: PASS.

- [ ] **Step 4: Record exact issue closure evidence**

The implementation PR lists the seven named recovery tests, confirms active-only and stranded-only preservation, exact-duplicate collapse, same-path single accounting, UUID/path-ordinal identity, hash-equivalence-only behavior, conflict/uninterpretable all-or-nothing abort, dry-run no writes, immutable stranded source, fresh V13 destination, independent verification, active backup, and same-filesystem atomic replacement. It also lists the migration/blocking/direct-read/executable-continuation tests, states every test passed without skips, and says: “Issue #31 remains open until Plan 2’s ingestion-half tests pass.”

- [ ] **Step 5: Commit only if closure assertions changed**

```bash
git add internal/compat/recovery_test.go internal/storage/recovery_destination_test.go internal/recovery/recovery_test.go cmd/backscroll/recover_test.go cmd/backscroll/index_policy_test.go cmd/backscroll/compat_diagnostics_test.go
git commit -m "test(recovery): prove active stranded atomic union"
```
