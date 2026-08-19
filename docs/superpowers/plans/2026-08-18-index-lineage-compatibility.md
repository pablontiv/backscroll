# Index Lineage Compatibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every published Go index lineage through v3.2.5 inspectable and losslessly upgradeable with shape-safe, transactional migration primitives.

**Architecture:** Add a narrow, stateless, read-only `internal/compat` inspector that classifies observed SQLite shape and returns typed diagnostics or ordered migration identifiers. Keep snapshots, transactions, migration SQL, and final verification in storage consumers. This slice deliberately does not activate stale-index refusal or expose a `recover` continuation; Plan 3 does both only after `recover` is registered and functional.

**Tech Stack:** Go 1.26.2, `database/sql`, `modernc.org/sqlite`, table-driven Go tests, checked-in JSON/SQL fixtures, Just

**Spec:** `docs/superpowers/specs/2026-08-18-systemic-index-compatibility-design.md`

## Global Constraints

- This is delivery Plan 1. The exact chain is Plan 1 inspection/migration primitives → Plan 3 recovery and blocking-policy activation → Plan 2 manifest ingestion → Plan 4 shipped guidance validation.
- `internal/compat` is stateless and read-only: no writes, transactions, backups, destination paths, Cobra output, retries, cached status, or workflow state.
- Plan 1 defines only schema-inspection and migration types required by this slice: `Code`, `Diagnostic`, `SchemaShape`, `MigrationStep`, `MigrationPlan`, and `Queryer`.
- Plan 1 does not define manifest plans, reader lookup, recovery inputs/plans, canonical recovery records, or move `storage.IndexedRecord`; those types belong to the first plan that consumes them.
- Actual tables, columns, indexes, triggers, and migration rows select a lineage; a migration version alone never does.
- Hermetic tests use only a checked-in release/schema manifest and fixtures covering published Go releases v0.3.7 through v3.2.5; they never consult ambient Git tags or the network.
- Frozen Rust v0 remains outside in-place migration.
- No user-facing command may refuse service or print `recover` in this slice. Unsupported shapes remain inspectable typed results for Plan 3 to render after the continuation exists.
- Every filesystem test uses `t.TempDir()` and no real HOME, config, Git tag state, or network.
- Follow strict RED → GREEN → TRIANGULATE → REFACTOR for every task. Run focused tests before `just check`, `just test`, and `just ci`.
- Commit commands below are instructions for a future explicitly authorized implementation. Do not stage or commit while authoring or reviewing this plan.
- Do not implement the reported large-file symptom without a focused fixture that fails on current code.

---

## File map

| Path | Responsibility |
|---|---|
| `internal/compat/types.go` | Core schema compatibility codes, diagnostics, shapes, migration plans, and the read-only query interface. |
| `internal/compat/catalog.go` | Embed and validate the checked-in release/schema inventory. |
| `internal/compat/catalog_test.go` | Hermetic inventory completeness and corruption tests. |
| `internal/compat/schema.go` | Read SQLite metadata and classify deterministic schema signatures. |
| `internal/compat/schema_test.go` | Shape, unsupported-lineage, and idempotency tests. |
| `internal/compat/testdata/release-schemas/manifest.json` | Hermetic v0.3.7–v3.2.5 release-to-fixture inventory and provenance checksums. |
| `internal/compat/testdata/release-schemas/*.sql` | Unique published V1–V13 shapes plus observed `source_metadata` variants. |
| `internal/storage/migration_plan.go` | Snapshot, execute named steps in one transaction, verify final shape, and reopen. |
| `internal/storage/migration_plan_test.go` | Lossless lineage, destructive snapshot, rollback, and final-shape tests. |
| `internal/storage/migrations.go` | Existing V1–V13 SQL bodies refactored into transaction-aware step helpers. |
| `internal/storage/storage.go` | Open a supported lineage through inspection and the safe migration executor. |

### Task 1: Define the core schema contract and hermetic release inventory

**Files:**
- Create: `internal/compat/types.go`
- Create: `internal/compat/catalog.go`
- Create: `internal/compat/catalog_test.go`
- Create: `internal/compat/testdata/release-schemas/manifest.json`
- Create: `internal/compat/testdata/release-schemas/v1.sql`
- Create: `internal/compat/testdata/release-schemas/v2.sql`
- Create: `internal/compat/testdata/release-schemas/v3.sql`
- Create: `internal/compat/testdata/release-schemas/v3-no-source-metadata.sql`
- Create: `internal/compat/testdata/release-schemas/v4.sql`
- Create: `internal/compat/testdata/release-schemas/v5-with-source-metadata.sql`
- Create: `internal/compat/testdata/release-schemas/v5-without-source-metadata.sql`
- Create: `internal/compat/testdata/release-schemas/v7.sql`
- Create: `internal/compat/testdata/release-schemas/v8.sql`
- Create: `internal/compat/testdata/release-schemas/v9.sql`
- Create: `internal/compat/testdata/release-schemas/v10.sql`
- Create: `internal/compat/testdata/release-schemas/v11.sql`
- Create: `internal/compat/testdata/release-schemas/v12.sql`
- Create: `internal/compat/testdata/release-schemas/v13.sql`

**Interfaces:**
- Consumes: existing `*sql.DB`, whose `QueryContext(context.Context, string, ...any) (*sql.Rows, error)` and `QueryRowContext(context.Context, string, ...any) *sql.Row` satisfy `compat.Queryer`.
- Produces: `Code`, `Diagnostic`, `SchemaShape`, `MigrationStep`, `MigrationPlan`, `Queryer`, `Catalog`, and `func LoadCatalog() (Catalog, error)`. No manifest or recovery types are introduced.

- [ ] **Step 1: Write the release inventory failure first**

```go
func TestCheckedInReleaseSchemaManifestIsComplete(t *testing.T) {
    catalog, err := LoadCatalog()
    if err != nil { t.Fatal(err) }
    if catalog.FirstGoRelease != "v0.3.7" || catalog.LatestGoRelease != "v3.2.5" {
        t.Fatalf("catalog bounds = %s..%s", catalog.FirstGoRelease, catalog.LatestGoRelease)
    }
    seen := map[string]bool{}
    for _, release := range catalog.Releases {
        if release.Tag == "" || release.Fixture == "" || release.ProvenanceSHA256 == "" || seen[release.Tag] {
            t.Fatalf("invalid release mapping: %+v", release)
        }
        seen[release.Tag] = true
        if _, err := fs.Stat(releaseSchemaFS, "testdata/release-schemas/"+release.Fixture); err != nil {
            t.Fatalf("fixture %q: %v", release.Fixture, err)
        }
    }
    if !seen["v0.3.7"] || !seen["v3.2.5"] { t.Fatalf("release endpoints missing: %v", seen) }
}
```

- [ ] **Step 2: Run RED and confirm the missing catalog is the reason**

Run: `go test ./internal/compat -run '^TestCheckedInReleaseSchemaManifestIsComplete$'`

Expected: FAIL to compile with `undefined: LoadCatalog` or fail because `manifest.json` is absent; it must not fail from HOME, Git, or network access.

- [ ] **Step 3: Add the minimal schema-only contract and embedded catalog**

```go
type Code string

const (
    CodeUnsupportedLineage Code = "unsupported_lineage"
    CodeMigrationFailed    Code = "migration_failed"
    CodeIndexStale         Code = "index_stale"
)

type Diagnostic struct {
    Code         Code
    Summary      string
    Continuation []string
}

type SchemaShape struct { AppliedVersion int; Signature string }
type MigrationStep struct { Version int; Name string }
type MigrationPlan struct { From SchemaShape; Steps []MigrationStep }

type Queryer interface {
    QueryContext(context.Context, string, ...any) (*sql.Rows, error)
    QueryRowContext(context.Context, string, ...any) *sql.Row
}
```

`Continuation` is present because later command policy consumes it, but Plan 1 never renders it and does not invent a `recover` argv. The catalog lists every published Go release tag individually and maps it to one exact fixture and SHA-256 provenance. Use the approved spans: V1 `v0.3.7–v0.3.10`, V2/V3 `v0.3.11–v1.3.5`, V4 `v1.4.0–v1.4.4`, V5 variants `v2.0.0–v2.1.0`, V7 `v2.2.0–v2.2.3`, V8 `v2.3.0`, V9 `v2.4.0–v2.5.0`, V10 `v2.6.0`, V11 `v2.7.0`, V12 `v2.8.0–v2.11.0`, and V13 `v2.12.0–v3.2.5`.

Because local tags stop at v2.16.1, an explicitly authorized maintainer capture may run `gh release list --repo pablontiv/backscroll --limit 200 --json tagName,publishedAt` once while authoring the checked-in manifest. No test, build, or CI command runs it.

- [ ] **Step 4: GREEN, then triangulate truncation and missing fixtures**

Run: `go test ./internal/compat -run '^(TestCheckedInReleaseSchemaManifestIsComplete|TestReleaseSchemaManifestRejectsMissingFixture|TestReleaseSchemaManifestRejectsLatestBeforeV3_2_5)$'`

Expected: PASS; tests replace the embedded FS with `fstest.MapFS` and reject a missing fixture and a latest bound below v3.2.5 without invoking Git or HTTP.

- [ ] **Step 5: Refactor and run package tests**

Run: `go test ./internal/compat`

Expected: PASS with no network or environment dependency.

- [ ] **Step 6: Commit the coherent inventory work unit**

```bash
git add internal/compat/types.go internal/compat/catalog.go internal/compat/catalog_test.go internal/compat/testdata/release-schemas
git commit -m "feat(compat): add hermetic release schema inventory"
```

### Task 2: Inspect real SQLite shape and return typed migration decisions

**Files:**
- Create: `internal/compat/schema.go`
- Create: `internal/compat/schema_test.go`
- Modify: `internal/compat/catalog.go`

**Interfaces:**
- Consumes: `Queryer` and embedded `Catalog` from Task 1.
- Produces: `func InspectIndex(ctx context.Context, q Queryer) (MigrationPlan, *Diagnostic, error)` and `func VerifyCurrentShape(ctx context.Context, q Queryer) error`.

- [ ] **Step 1: Write table-driven shape tests**

```go
func TestInspectIndexUsesObservedShapeNotVersionAlone(t *testing.T) {
    tests := []struct{ fixture, wantFirstStep string }{
        {"v5-with-source-metadata.sql", "V6 drop source_metadata when present"},
        {"v5-without-source-metadata.sql", "V7 reasoning triggers"},
    }
    for _, tt := range tests { t.Run(tt.fixture, func(t *testing.T) {
        db := openFixtureCopy(t, tt.fixture)
        plan, diag, err := InspectIndex(context.Background(), db)
        if err != nil || diag != nil { t.Fatalf("plan error=%v diagnostic=%+v", err, diag) }
        if len(plan.Steps) == 0 || plan.Steps[0].Name != tt.wantFirstStep { t.Fatalf("steps=%+v", plan.Steps) }
    }) }
}
```

Also add `TestInspectIndexCurrentShapeIsIdempotent` and `TestInspectIndexUnsupportedShapeReturnsInternalDiagnostic`. The unsupported-shape assertion requires `CodeUnsupportedLineage`, the observed signature in `Summary`, and an empty `Continuation`; Plan 3 supplies executable argv only when it activates command policy.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/compat -run '^TestInspectIndex'`

Expected: FAIL with `undefined: InspectIndex`.

- [ ] **Step 3: Implement deterministic metadata inspection**

Query `schema_migrations`, `sqlite_master`, `PRAGMA table_info(<quoted-table>)`, `PRAGMA index_list(<quoted-table>)`, `PRAGMA index_info(<quoted-index>)`, and relevant trigger SQL. Canonicalize by sorting records as `kind|table|name|columns|sql`, exclude volatile SQLite metadata, hash with SHA-256, and match only the checked-in catalog.

```go
func InspectIndex(ctx context.Context, q Queryer) (MigrationPlan, *Diagnostic, error) {
    shape, err := inspectShape(ctx, q)
    if err != nil { return MigrationPlan{}, nil, fmt.Errorf("inspect schema: %w", err) }
    lineage, ok := defaultCatalog.BySignature(shape.Signature)
    if !ok {
        return MigrationPlan{}, &Diagnostic{
            Code: CodeUnsupportedLineage,
            Summary: fmt.Sprintf("unsupported index schema %s", shape.Signature),
        }, nil
    }
    return MigrationPlan{From: shape, Steps: lineage.RemainingSteps()}, nil, nil
}
```

- [ ] **Step 4: GREEN and triangulate corrupt metadata**

Run: `go test ./internal/compat -run '^(TestInspectIndex|TestVerifyCurrentShape)'`

Expected: PASS; malformed SQLite returns a wrapped Go error, while a readable unknown schema returns `CodeUnsupportedLineage` without user-facing command text.

- [ ] **Step 5: Refactor canonicalization and rerun the package**

Run: `go test ./internal/compat`

Expected: PASS; metadata order changes do not change signatures.

- [ ] **Step 6: Commit the inspector**

```bash
git add internal/compat/schema.go internal/compat/schema_test.go internal/compat/catalog.go
git commit -m "feat(compat): inspect observed index schema lineage"
```

### Task 3: Execute migration plans with snapshot, one transaction, and independent verification

**Files:**
- Create: `internal/storage/migration_plan.go`
- Create: `internal/storage/migration_plan_test.go`
- Modify: `internal/storage/migrations.go:8-175`
- Modify: `internal/storage/storage.go:20-47`

**Interfaces:**
- Consumes: `compat.InspectIndex`, `compat.MigrationPlan`, and existing V1–V13 SQL bodies in `migrations.go`.
- Produces: `func OpenCompatible(ctx context.Context, path string) (*Database, *compat.Diagnostic, error)`, `func (d *Database) ApplyMigrationPlan(ctx context.Context, path string, plan compat.MigrationPlan) error`, and `func SnapshotDatabase(ctx context.Context, srcPath string) (string, error)`.

- [ ] **Step 1: Write lossless and rollback tests first**

Add named tests `TestPublishedGoLineagesUpgradeLosslessly`, `TestHistoricalLineageWithoutSourceMetadataUpgradesLosslessly`, and `TestMigrationSnapshotAndRollbackOnDestructiveFailure`. Copy each SQL fixture into `t.TempDir()`, seed sentinel rows, and assert row identity/count plus FTS queryability after migration. Inject failure after the first destructive step and assert the original database remains at its pre-transaction shape and the sibling snapshot reopens read-only.

- [ ] **Step 2: Run RED**

Run: `go test ./internal/storage -run '^(TestPublishedGoLineagesUpgradeLosslessly|TestHistoricalLineageWithoutSourceMetadataUpgradesLosslessly|TestMigrationSnapshotAndRollbackOnDestructiveFailure)$'`

Expected: FAIL because `Open` still runs migrations independently and V6 fails on the missing-column fixture.

- [ ] **Step 3: Implement minimal migration execution**

Refactor migration bodies into transaction-aware helpers with exact shape `func applyV6(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error`; preserve existing SQL and migration names. `ApplyMigrationPlan` creates and fsyncs a sibling snapshot before the first destructive step, begins one transaction, dispatches only known `(Version, Name)` pairs, calls `compat.VerifyCurrentShape(ctx, tx)`, and commits once. Reopen read-only and verify current shape after commit before returning success.

```go
func OpenCompatible(ctx context.Context, path string) (*Database, *compat.Diagnostic, error) {
    inspect, err := OpenReadOnly(path)
    if errors.Is(err, fs.ErrNotExist) {
        db, openErr := Open(path)
        return db, nil, openErr
    }
    if err != nil { return nil, nil, err }
    plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
    _ = inspect.Close()
    if err != nil || diag != nil { return nil, diag, err }
    db, err := openWithoutSetup(path)
    if err == nil && len(plan.Steps) > 0 { err = db.ApplyMigrationPlan(ctx, path, plan) }
    return db, nil, err
}
```

`OpenCompatible` is a migration primitive only. No existing command is switched to it in Plan 1, so this slice cannot introduce a refusal with a nonexistent continuation.

- [ ] **Step 4: GREEN and triangulate final-shape failure**

Run: `go test ./internal/storage -run '^(TestPublishedGoLineagesUpgradeLosslessly|TestHistoricalLineageWithoutSourceMetadataUpgradesLosslessly|TestMigrationSnapshotAndRollbackOnDestructiveFailure|TestMigrationFinalShapeFailureRollsBack)$'`

Expected: PASS; injected final-shape failure occurs before commit and leaves all seeded rows unchanged.

- [ ] **Step 5: Refactor without widening compatibility scope**

Run: `go test ./internal/storage`

Expected: PASS; existing V8–V13 migration tests remain green.

- [ ] **Step 6: Run the Plan 1 boundary and repository gates**

Run: `go test ./internal/compat ./internal/storage -run '^(TestCheckedInReleaseSchemaManifestIsComplete|TestPublishedGoLineagesUpgradeLosslessly|TestHistoricalLineageWithoutSourceMetadataUpgradesLosslessly|TestMigrationSnapshotAndRollbackOnDestructiveFailure|TestMigrationFinalShapeFailureRollsBack)$'`

Expected: PASS with no skipped tests and no Cobra/recovery dependency.

Run: `just check`

Expected: PASS.

Run: `just test`

Expected: PASS.

Run: `just ci`

Expected: PASS.

- [ ] **Step 7: Record the safe intermediate boundary and commit**

The implementation PR states: “Plan 1 provides inspected, shape-safe migration primitives only. It does not activate stale-index refusal, emit `recover`, or close issue #31. Plan 3 activates the policy after recovery exists; Plan 2 supplies the ingestion half required to close #31.”

```bash
git add internal/storage/migration_plan.go internal/storage/migration_plan_test.go internal/storage/migrations.go internal/storage/storage.go
git commit -m "feat(storage): migrate supported lineages atomically"
```
