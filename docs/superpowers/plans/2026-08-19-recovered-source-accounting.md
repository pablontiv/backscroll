# Recovered Source Accounting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every successfully recovered database pass public validation while preserving backfill eligibility and forcing any still-present source to be genuinely re-synced.

**Architecture:** Recovery writes one provisional `indexed_files` row per distinct recovered `source_path`, using the reserved non-SHA marker `backscroll:recovered` and a NULL `last_indexed`. Recovery verification requires an exact marker set; hash- and status-oriented consumers exclude markers, backfill includes them, and the existing `SyncFiles` upsert replaces a marker with a real hash.

**Tech Stack:** Go, cobra command tests, `database/sql`, modernc.org/sqlite, SQLite FTS5, stdlib `testing`.

**Spec:** `docs/superpowers/specs/2026-08-19-recovered-source-accounting-design.md`

## Global Constraints

- Recovery reconstructs database state only; it must never create, restore, read, or modify original JSONL or Markdown source files.
- The provisional hash value is exactly `backscroll:recovered` and must never look like a 64-character hexadecimal SHA-256.
- Provisional rows use `NULL` for `last_indexed`.
- Public validation must continue rejecting genuine `search_items` rows with no matching accounting path.
- Recovered paths remain eligible for `BackfillDerived()`.
- `GetFileHashes()` and `GetStats()` must not treat provisional rows as sources indexed from disk.
- A normal `SyncFiles()` call must replace provisional accounting with the real hash in the same transaction as the synced records.
- Do not add a migration or change the SQLite schema.
- Preserve deterministic recovery output, canonical records, FTS queryability, atomic installation, and byte-identical active backup behavior.

## File Structure

- Create `internal/storage/recovery_accounting.go`: owns the reserved marker and the predicate that identifies provisional source accounting.
- Modify `internal/storage/recovery_destination.go`: writes deterministic provisional accounting and verifies its exact committed shape.
- Modify `internal/storage/recovery_destination_test.go`: pins marker creation, NULL timestamp, exact-path verification, and tamper rejection.
- Modify `internal/storage/sync.go`: excludes provisional markers from the autosync hash skip map; normal sync upsert remains the transition mechanism.
- Modify `internal/storage/queries.go`: excludes provisional markers from file-count and last-indexed statistics.
- Modify `internal/storage/backfill.go`: treats provisional markers like missing/expired source accounting.
- Modify `internal/storage/storage_test.go`: tests hash-map filtering, status filtering, and replacement by normal sync.
- Modify `internal/storage/backfill_test.go`: proves marked recovered paths remain eligible for derived-data backfill.
- Modify `cmd/backscroll/recover_test.go`: adds the end-to-end v13 + v7 recovery-to-validation regression and verifies backup/rows/FTS.

---

### Task 1: Write and verify provisional recovery accounting

**Files:**
- Create: `internal/storage/recovery_accounting.go`
- Modify: `internal/storage/recovery_destination.go:116-136, 230-305`
- Test: `internal/storage/recovery_destination_test.go:330-460, 660-725`

**Interfaces:**
- Consumes: `compat.RecoveryPlan`, `compat.CanonicalRecord.Record.SourcePath`, the existing recovery transaction, and `compat.Queryer`.
- Produces: `const recoveredSourceHash = "backscroll:recovered"`, `func isRecoveredSourceHash(string) bool`, `func recoveryDestinationSourcePaths(compat.RecoveryPlan) []string`, `func insertRecoveryDestinationAccounting(context.Context, *sql.Tx, compat.RecoveryPlan) error`, and exact accounting checks inside `verifyRecoveryDestinationRows`.

- [ ] **Step 1: Add failing destination-creation assertions**

Update `TestRecoveryDestinationStartsFreshAtCurrentSchema` and `assertRecoveryDestinationRecords` so they expect one marker per distinct planned path instead of zero `indexed_files` rows. Use a query that also verifies `last_indexed` is NULL:

```go
rows, err := db.DB().Query(`
    SELECT path, hash, last_indexed
    FROM indexed_files
    ORDER BY path
`)
if err != nil {
    t.Fatalf("query recovered source accounting: %v", err)
}
defer func() { _ = rows.Close() }()

var gotPaths []string
for rows.Next() {
    var path, hash string
    var lastIndexed sql.NullString
    if err := rows.Scan(&path, &hash, &lastIndexed); err != nil {
        t.Fatalf("scan recovered source accounting: %v", err)
    }
    if hash != recoveredSourceHash || lastIndexed.Valid {
        t.Fatalf("accounting for %s = hash %q last_indexed %+v", path, hash, lastIndexed)
    }
    gotPaths = append(gotPaths, path)
}
```

Build `wantPaths` from the plan's distinct `SourcePath` values, sort it, and compare with `reflect.DeepEqual(gotPaths, wantPaths)`. Add `database/sql` to the test imports if it is not already present.

- [ ] **Step 2: Add failing tamper cases for missing and incorrect accounting**

Add a table-driven test next to `TestRecoveryDestinationIndependentVerificationRejectsTamper`:

```go
func TestRecoveryDestinationVerificationRejectsInvalidRecoveredAccounting(t *testing.T) {
    cases := []struct {
        name   string
        mutate string
    }{
        {
            name:   "missing path",
            mutate: `DELETE FROM indexed_files WHERE path = '/sessions/source.jsonl';`,
        },
        {
            name:   "real-looking but unverified hash",
            mutate: `UPDATE indexed_files SET hash = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' WHERE path = '/sessions/source.jsonl';`,
        },
        {
            name:   "non-null last indexed",
            mutate: `UPDATE indexed_files SET last_indexed = '2026-08-19T00:00:00Z' WHERE path = '/sessions/source.jsonl';`,
        },
    }
    // For every case, create a one-path recovery destination, mutate it,
    // and require VerifyRecoveryDestination(ctx, destPath, plan) to fail.
}
```

Keep the existing invented-extra-path tamper test; it covers the fourth exact-set failure mode.

- [ ] **Step 3: Run the focused tests and confirm RED**

Run:

```bash
go test ./internal/storage -run 'TestRecoveryDestination(Start|Verification|Independent)' -count=1
```

Expected: FAIL because recovery still produces zero accounting rows and the new marker identifiers do not exist.

- [ ] **Step 4: Create the marker contract**

Create `internal/storage/recovery_accounting.go`:

```go
package storage

const recoveredSourceHash = "backscroll:recovered"

func isRecoveredSourceHash(hash string) bool {
    return hash == recoveredSourceHash
}
```

Keep the marker unexported because it is an internal storage representation, not CLI or package API.

- [ ] **Step 5: Insert deterministic provisional accounting**

In `internal/storage/recovery_destination.go`, add these helpers:

```go
func recoveryDestinationSourcePaths(plan compat.RecoveryPlan) []string {
    seen := make(map[string]struct{}, len(plan.Records))
    for _, planned := range plan.Records {
        seen[planned.Record.SourcePath] = struct{}{}
    }
    paths := make([]string, 0, len(seen))
    for path := range seen {
        paths = append(paths, path)
    }
    sort.Strings(paths)
    return paths
}

func insertRecoveryDestinationAccounting(ctx context.Context, tx *sql.Tx, plan compat.RecoveryPlan) error {
    for _, path := range recoveryDestinationSourcePaths(plan) {
        if _, err := tx.ExecContext(ctx, `
            INSERT INTO indexed_files (path, hash, last_indexed)
            VALUES (?, ?, NULL)
        `, path, recoveredSourceHash); err != nil {
            return fmt.Errorf("insert recovered source accounting for %s: %w", path, err)
        }
    }
    return nil
}
```

Call `insertRecoveryDestinationAccounting(ctx, tx, plan)` immediately after the canonical record insertion loop and before `verifyRecoveryDestinationQueryer`. Wrap failure as `insert recovery source accounting: %w`.

- [ ] **Step 6: Replace the zero-row verifier with exact marker verification**

In `verifyRecoveryDestinationRows`, remove the `COUNT(*) == 0` block. Query all accounting rows ordered by path, scan `last_indexed` as `sql.NullString`, and compare against `recoveryDestinationSourcePaths(plan)`:

```go
rows, err := q.QueryContext(ctx, `
    SELECT path, hash, last_indexed
    FROM indexed_files
    ORDER BY path
`)
if err != nil {
    return fmt.Errorf("query recovered source accounting: %w", err)
}

var gotPaths []string
for rows.Next() {
    var path, hash string
    var lastIndexed sql.NullString
    if err := rows.Scan(&path, &hash, &lastIndexed); err != nil {
        _ = rows.Close()
        return fmt.Errorf("scan recovered source accounting: %w", err)
    }
    if !isRecoveredSourceHash(hash) {
        _ = rows.Close()
        return fmt.Errorf("recovered source accounting for %s has hash %q", path, hash)
    }
    if lastIndexed.Valid {
        _ = rows.Close()
        return fmt.Errorf("recovered source accounting for %s has last_indexed %q", path, lastIndexed.String)
    }
    gotPaths = append(gotPaths, path)
}
if err := rows.Err(); err != nil {
    _ = rows.Close()
    return fmt.Errorf("read recovered source accounting: %w", err)
}
if err := rows.Close(); err != nil {
    return fmt.Errorf("close recovered source accounting: %w", err)
}
wantPaths := recoveryDestinationSourcePaths(plan)
if !reflect.DeepEqual(gotPaths, wantPaths) {
    return fmt.Errorf("recovered source accounting paths do not match plan")
}
```

Do not add `reflect` solely for this comparison if a small existing slice-equality helper is clearer; if using `reflect.DeepEqual`, add the import explicitly. Ensure rows are closed before later queries on the single SQLite connection.

- [ ] **Step 7: Run destination tests and confirm GREEN**

Run:

```bash
go test ./internal/storage -run 'TestRecoveryDestination|TestPublishedGoLineagesUpgradeLosslessly' -count=1
```

Expected: PASS, including the existing canonical-row, FTS, cleanup, and tamper tests.

- [ ] **Step 8: Commit Task 1**

```bash
git add internal/storage/recovery_accounting.go internal/storage/recovery_destination.go internal/storage/recovery_destination_test.go
git commit -m "fix(storage): account for recovered source paths"
```

---

### Task 2: Make sync, status, and backfill marker-aware

**Files:**
- Modify: `internal/storage/sync.go:320-342`
- Modify: `internal/storage/queries.go:25-48`
- Modify: `internal/storage/backfill.go:50-65`
- Test: `internal/storage/storage_test.go:350-425`
- Test: `internal/storage/backfill_test.go:1-50, 250-295`

**Interfaces:**
- Consumes: `isRecoveredSourceHash(string) bool` and `recoveredSourceHash` from Task 1.
- Produces: `GetFileHashes()` containing only real source hashes, `GetStats()` containing only genuinely indexed file accounting, and `BackfillDerived()` eligibility for missing or provisionally recovered paths. `SyncFiles([]IndexedFile)` remains unchanged and replaces markers through its existing upsert.

- [ ] **Step 1: Add failing hash-map and stats tests**

Extend `TestGetFileHashes` after the normal sync:

```go
if _, err := db.db.Exec(`
    INSERT INTO indexed_files (path, hash, last_indexed)
    VALUES ('/path/to/recovered.jsonl', ?, NULL)
`, recoveredSourceHash); err != nil {
    t.Fatalf("insert recovered accounting: %v", err)
}

hashes, err := db.GetFileHashes()
if err != nil {
    t.Fatalf("failed to get hashes: %v", err)
}
if _, ok := hashes["/path/to/recovered.jsonl"]; ok {
    t.Fatalf("GetFileHashes returned provisional recovered path")
}
```

Add `TestGetStatsExcludesRecoveredSourceAccounting`:

```go
func TestGetStatsExcludesRecoveredSourceAccounting(t *testing.T) {
    db, cleanup := newTestDB(t)
    defer cleanup()

    if _, err := db.db.Exec(`
        INSERT INTO indexed_files (path, hash, last_indexed) VALUES
        ('/real.jsonl', 'real-hash', '2026-08-18T12:00:00Z'),
        ('/recovered.jsonl', ?, NULL)
    `, recoveredSourceHash); err != nil {
        t.Fatal(err)
    }
    stats, err := db.GetStats()
    if err != nil {
        t.Fatal(err)
    }
    if stats.TotalFiles != 1 {
        t.Fatalf("TotalFiles = %d, want 1", stats.TotalFiles)
    }
    want := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
    if !stats.IndexedAt.Equal(want) {
        t.Fatalf("IndexedAt = %s, want %s", stats.IndexedAt, want)
    }
}
```

- [ ] **Step 2: Add a normal-sync replacement regression**

Add `database/sql` to `storage_test.go` imports, then add `TestSyncFilesReplacesRecoveredSourceAccounting`:

```go
func TestSyncFilesReplacesRecoveredSourceAccounting(t *testing.T) {
    db, cleanup := newTestDB(t)
    defer cleanup()

    const path = "/path/to/recovered.jsonl"
    if _, err := db.db.Exec(`
        INSERT INTO indexed_files (path, hash, last_indexed)
        VALUES (?, ?, NULL)
    `, path, recoveredSourceHash); err != nil {
        t.Fatal(err)
    }
    err := db.SyncFiles([]IndexedFile{{
        SourcePath: path,
        Source:     "session",
        Hash:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
        Project:    "proj",
        Messages: []IndexedMessage{{
            Ordinal: 0, Role: "user", Text: "resynced", UUID: getTestUUID(), ContentType: "text",
        }},
    }})
    if err != nil {
        t.Fatal(err)
    }
    var hash string
    var lastIndexed sql.NullString
    if err := db.db.QueryRow(`SELECT hash, last_indexed FROM indexed_files WHERE path = ?`, path).Scan(&hash, &lastIndexed); err != nil {
        t.Fatal(err)
    }
    if hash == recoveredSourceHash || !lastIndexed.Valid {
        t.Fatalf("accounting after sync = hash %q last_indexed %+v", hash, lastIndexed)
    }
}
```

This test should already pass once the marker exists, proving that no special `SyncFiles` branch is necessary.

Also add `TestPurgeRemovesRecoveredSourceAccounting` to pin the final lifecycle transition:

```go
func TestPurgeRemovesRecoveredSourceAccounting(t *testing.T) {
    db, cleanup := newTestDB(t)
    defer cleanup()

    const path = "/path/to/purged-recovery.jsonl"
    if _, err := db.db.Exec(`
        INSERT INTO search_items
        (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
        VALUES ('session', ?, 0, 'user', 'old recovered row', '2026-01-01T00:00:00Z',
                '33333333-3333-4333-8333-333333333333', 'proj', 'text')
    `, path); err != nil {
        t.Fatal(err)
    }
    if _, err := db.db.Exec(`
        INSERT INTO indexed_files (path, hash, last_indexed)
        VALUES (?, ?, NULL)
    `, path, recoveredSourceHash); err != nil {
        t.Fatal(err)
    }
    if _, err := db.Purge("2026-01-02T00:00:00Z"); err != nil {
        t.Fatal(err)
    }
    var count int
    if err := db.db.QueryRow(`SELECT COUNT(*) FROM indexed_files WHERE path = ?`, path).Scan(&count); err != nil {
        t.Fatal(err)
    }
    if count != 0 {
        t.Fatalf("recovered accounting survived purge")
    }
}
```

- [ ] **Step 3: Add a failing marked-path backfill test**

Add `TestBackfillDerivedMinesRecoveredMarkedPath` beside `TestBackfillDerivedMinesTemplatesFromExpiredFile`:

```go
func TestBackfillDerivedMinesRecoveredMarkedPath(t *testing.T) {
    db, err := Open(filepath.Join(t.TempDir(), "test.db"))
    if err != nil {
        t.Fatal(err)
    }
    defer func() { _ = db.Close() }()

    if _, err := db.db.Exec(`
        INSERT INTO search_items
        (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version)
        VALUES ('session', '/recovered/s.jsonl', 0, 'assistant', 'error: recovered failure 42',
                '2026-01-01T00:00:00Z', 'recovered#t0', 'proj', 'tool', 1)
    `); err != nil {
        t.Fatal(err)
    }
    if _, err := db.db.Exec(`
        INSERT INTO indexed_files (path, hash, last_indexed)
        VALUES ('/recovered/s.jsonl', ?, NULL)
    `, recoveredSourceHash); err != nil {
        t.Fatal(err)
    }

    if err := db.BackfillDerived(BackfillDerivedOpts{}); err != nil {
        t.Fatal(err)
    }
    var count int
    if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&count); err != nil {
        t.Fatal(err)
    }
    if count == 0 {
        t.Fatal("marked recovered path was excluded from backfill")
    }
}
```

- [ ] **Step 4: Run the focused tests and confirm RED where behavior is missing**

Run:

```bash
go test ./internal/storage -run 'Test(GetFileHashes|GetStatsExcludesRecovered|SyncFilesReplacesRecovered|PurgeRemovesRecovered|BackfillDerivedMinesRecovered)' -count=1
```

Expected: hash-map, stats, and backfill tests FAIL; sync replacement PASS demonstrates the existing transition.

- [ ] **Step 5: Filter provisional rows from the autosync skip map**

Change `GetFileHashes` in `internal/storage/sync.go`:

```go
rows, err := d.db.Query(`
    SELECT path, hash
    FROM indexed_files
    WHERE hash <> ?
`, recoveredSourceHash)
```

Keep its return type and scan/error behavior unchanged.

- [ ] **Step 6: Filter provisional rows from status statistics**

Change both `GetStats` queries in `internal/storage/queries.go`:

```go
err := d.db.QueryRow(`
    SELECT COUNT(*)
    FROM indexed_files
    WHERE hash <> ?
`, recoveredSourceHash).Scan(&stats.TotalFiles)
```

```go
err = d.db.QueryRow(`
    SELECT MAX(last_indexed)
    FROM indexed_files
    WHERE hash <> ?
`, recoveredSourceHash).Scan(&lastIndexed)
```

Do not alter message, chunk, embedding, or vector counts.

- [ ] **Step 7: Include provisional paths in expired/recovery backfill discovery**

Change the `BackfillDerived` discovery predicate and pass the marker as a parameter:

```go
rows, err := d.db.Query(`
    SELECT DISTINCT si.source_path, si.source
    FROM search_items si
    LEFT JOIN indexed_files ifx ON si.source_path = ifx.path
    WHERE
        (ifx.path IS NULL OR ifx.hash = ?) AND
        (NOT EXISTS (SELECT 1 FROM template_matches WHERE source_path = si.source_path) OR
         NOT EXISTS (SELECT 1 FROM correction_signals WHERE source_path = si.source_path) OR
         NOT EXISTS (SELECT 1 FROM tool_events WHERE source_path = si.source_path AND extraction_version = 0))
    ORDER BY si.source_path
`, recoveredSourceHash)
```

Keep the existing stale-template merge, batching, idempotency predicates, and transaction boundaries unchanged.

- [ ] **Step 8: Run marker-consumer and broader storage tests**

Run:

```bash
go test ./internal/storage -run 'Test(GetFileHashes|GetStats|SyncFiles|BackfillDerived|Purge|Validate)' -count=1
```

Expected: PASS. In particular, existing `TestBackfillDerivedSkipsOnDiskFiles` must still prove that a real hash excludes an on-disk path from lossy backfill.

- [ ] **Step 9: Commit Task 2**

```bash
git add internal/storage/sync.go internal/storage/queries.go internal/storage/backfill.go internal/storage/storage_test.go internal/storage/backfill_test.go
git commit -m "fix(storage): distinguish recovered source accounting"
```

---

### Task 3: Add the CLI recovery-to-validation regression

**Files:**
- Modify: `cmd/backscroll/recover_test.go:1-15, 145-225, 370-425`
- Read fixture: `tests/fixtures/recovery/active-v13.sql`
- Read fixture: `tests/fixtures/recovery/stranded-v7.sql`

**Interfaces:**
- Consumes: the public `recover --from` and `validate --indexed-only` commands, supported v13/v7 fixture schemas, and recovery output's `backup path: ` field.
- Produces: a hermetic regression proving the installed recovery destination is valid, complete, FTS-queryable, and backed up byte-for-byte.

- [ ] **Step 1: Add a fixture database helper**

Add `database/sql` to imports. Add this helper near `createRecoverTestDB`:

```go
func createRecoverFixtureDB(t *testing.T, path, fixture string, replacements map[string]string) {
    t.Helper()
    fixturePath := filepath.Join("..", "..", "tests", "fixtures", "recovery", fixture)
    body, err := os.ReadFile(fixturePath)
    if err != nil {
        t.Fatalf("read recovery fixture %s: %v", fixturePath, err)
    }
    sqlText := string(body)
    for from, to := range replacements {
        sqlText = strings.ReplaceAll(sqlText, from, to)
    }
    db, err := sql.Open("sqlite", path)
    if err != nil {
        t.Fatalf("open recovery fixture %s: %v", path, err)
    }
    if _, err := db.Exec(sqlText); err != nil {
        _ = db.Close()
        t.Fatalf("execute recovery fixture %s: %v", fixture, err)
    }
    if err := db.Close(); err != nil {
        t.Fatalf("close recovery fixture %s: %v", path, err)
    }
}
```

The `storage` package imported by this test already registers the modernc SQLite driver. Do not modify fixture files on disk.

- [ ] **Step 2: Add the end-to-end regression**

Add `TestRecoverInstalledDestinationPassesIndexedValidation`:

```go
func TestRecoverInstalledDestinationPassesIndexedValidation(t *testing.T) {
    dir := t.TempDir()
    home := filepath.Join(dir, "home")
    if err := os.MkdirAll(home, 0o755); err != nil {
        t.Fatal(err)
    }
    activePath := filepath.Join(dir, "active.db")
    strandedPath := filepath.Join(dir, "stranded.db")
    createRecoverFixtureDB(t, activePath, "active-v13.sql", map[string]string{
        "uuid-active-v13": "11111111-1111-4111-8111-111111111111",
    })
    createRecoverFixtureDB(t, strandedPath, "stranded-v7.sql", map[string]string{
        "uuid-stranded-v7": "22222222-2222-4222-8222-222222222222",
    })
    activeBefore, err := os.ReadFile(activePath)
    if err != nil {
        t.Fatal(err)
    }

    t.Setenv("HOME", home)
    t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
    t.Setenv("BACKSCROLL_DATABASE_PATH", activePath)
    t.Chdir(dir)

    var recoverOut, recoverErr bytes.Buffer
    recoverCmd := buildRootCmd(&recoverOut, &recoverErr)
    recoverCmd.SetArgs([]string{"recover", "--from", strandedPath})
    if err := recoverCmd.Execute(); err != nil {
        t.Fatalf("recover: %v\nstderr=%s", err, recoverErr.String())
    }
    if recoverErr.String() != "" {
        t.Fatalf("recover stderr = %q, want empty", recoverErr.String())
    }
    backupPath := recoverOutputValue(t, recoverOut.String(), "backup path: ")
    backupBytes, err := os.ReadFile(backupPath)
    if err != nil {
        t.Fatal(err)
    }
    if !bytes.Equal(backupBytes, activeBefore) {
        t.Fatal("active backup differs from pre-recovery database")
    }

    var validateOut, validateErr bytes.Buffer
    validateCmd := buildRootCmd(&validateOut, &validateErr)
    validateCmd.SetArgs([]string{"validate", "--indexed-only"})
    if err := validateCmd.Execute(); err != nil {
        t.Fatalf("validate recovered destination: %v\nstdout=%s\nstderr=%s", err, validateOut.String(), validateErr.String())
    }
    if validateErr.String() != "" {
        t.Fatalf("validate stderr = %q, want empty", validateErr.String())
    }

    db, err := storage.OpenReadOnly(activePath)
    if err != nil {
        t.Fatal(err)
    }
    defer func() { _ = db.Close() }()
    var records, accountedPaths, ftsHits int
    if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items`).Scan(&records); err != nil {
        t.Fatal(err)
    }
    if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files`).Scan(&accountedPaths); err != nil {
        t.Fatal(err)
    }
    if err := db.DB().QueryRow(`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'default'`).Scan(&ftsHits); err != nil {
        t.Fatal(err)
    }
    if records != 4 || accountedPaths != 4 || ftsHits != 2 {
        t.Fatalf("recovered database counts records=%d paths=%d fts=%d, want 4/4/2", records, accountedPaths, ftsHits)
    }
}
```

The expected accounting count is four because the two fixtures contain four distinct `source_path` values. Retain command output in failure messages so a regression is diagnosable.

- [ ] **Step 3: Run the end-to-end regression**

Run:

```bash
go test ./cmd/backscroll -run TestRecoverInstalledDestinationPassesIndexedValidation -count=1
```

Expected: PASS. The RED behavior is already pinned by Task 1's failing destination-accounting tests: on the original implementation, recovery produced zero accounting rows and public validation reported four orphaned `search_items`.

- [ ] **Step 4: Run all recovery and compatibility command tests**

Run:

```bash
go test ./cmd/backscroll -run 'TestRecover|TestValidateCurrentIndexIntegrityFailureIsGenericAndReadOnly' -count=1
```

Expected: PASS. The existing genuine-orphan test must continue reporting `orphaned search_items`; recovery diagnostics and backup behavior must remain unchanged.

- [ ] **Step 5: Commit Task 3**

```bash
git add cmd/backscroll/recover_test.go
git commit -m "test(recovery): validate installed recovered index"
```

---

### Task 4: Run the complete quality gate and review the diff

**Files:**
- Verify only; modify earlier files only if a failing gate reveals a defect.

**Interfaces:**
- Consumes: all Task 1-3 commits.
- Produces: a verified implementation satisfying issue #35 and the repository's aggregate coverage gate.

- [ ] **Step 1: Check formatting and static analysis**

Run:

```bash
just check
```

Expected: PASS (`gofmt --check` and `go vet`). If formatting fails, run `just fmt`, inspect the diff, and recommit only the formatting changes with the task that introduced them.

- [ ] **Step 2: Run the full test suite**

Run:

```bash
just test
```

Expected: PASS for every package.

- [ ] **Step 3: Run the release-blocking CI mirror**

Run:

```bash
just ci
```

Expected: build succeeds, scrubbed-HOME tests pass, race checks pass, and aggregate statement coverage is at least 85%.

- [ ] **Step 4: Review the final diff against the spec**

Run:

```bash
git diff HEAD~3..HEAD --check
git diff HEAD~3..HEAD --stat
git log --oneline -4
```

Confirm all of the following from tests and code:

- recovery writes exact marked accounting with NULL `last_indexed`;
- validation still uses path presence and rejects genuine orphans;
- hashes and status exclude markers;
- backfill includes markers;
- normal sync replaces markers;
- purge uses its existing last-row cleanup;
- no migration, schema edit, JSONL write, or unrelated refactor was added.

- [ ] **Step 5: Record any gate-only fix**

If Step 1-4 required a code change, rerun the narrow failing test and all three quality commands, then commit with a precise conventional message such as:

```bash
git add internal/storage/recovery_accounting.go internal/storage/recovery_destination.go internal/storage/recovery_destination_test.go internal/storage/sync.go internal/storage/queries.go internal/storage/backfill.go internal/storage/storage_test.go internal/storage/backfill_test.go cmd/backscroll/recover_test.go
git commit -m "fix(storage): preserve recovered accounting invariant"
```

If no change was required, do not create an empty commit.
