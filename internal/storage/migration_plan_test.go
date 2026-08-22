package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
)

func TestCatalogGoLineagesUpgradeLosslessly(t *testing.T) {
	catalog, err := compat.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	var fixtures []struct {
		name              string
		appliedVersion    int
		expectSnapshot    bool
		hasSourceMetadata bool
	}
	for _, release := range catalog.Releases {
		if seen[release.Fixture] {
			continue
		}
		seen[release.Fixture] = true
		fixtures = append(fixtures, struct {
			name              string
			appliedVersion    int
			expectSnapshot    bool
			hasSourceMetadata bool
		}{
			name:              release.Fixture,
			appliedVersion:    release.AppliedVersion,
			expectSnapshot:    fixtureNeedsDestructiveSnapshot(release.AppliedVersion, release.HasSourceMetadata),
			hasSourceMetadata: release.HasSourceMetadata,
		})
	}
	for _, fixture := range catalog.UnmanifestedFixtures {
		if seen[fixture.Fixture] {
			continue
		}
		seen[fixture.Fixture] = true
		fixtures = append(fixtures, struct {
			name              string
			appliedVersion    int
			expectSnapshot    bool
			hasSourceMetadata bool
		}{
			name:              fixture.Fixture,
			appliedVersion:    fixture.AppliedVersion,
			expectSnapshot:    fixtureNeedsDestructiveSnapshot(fixture.AppliedVersion, fixture.HasSourceMetadata),
			hasSourceMetadata: fixture.HasSourceMetadata,
		})
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			dbPath := createFixtureDatabase(t, fixture.name)
			want := seedFixtureSentinels(t, dbPath)

			db, diag, err := OpenCompatible(context.Background(), dbPath)
			if err != nil || diag != nil {
				t.Fatalf("open compatible error=%v diagnostic=%+v", err, diag)
			}
			defer func() { _ = db.Close() }()

			assertSearchItems(t, db.DB(), want.SearchItems)
			assertSearchItemsByUUID(t, db.DB(), want.ToolSearchItems)
			assertTableSentinels(t, db.DB(), want)
			assertFTSQueryable(t, db.DB(), "sentinelterm", len(want.SearchItems))
			assertFTSQueryable(t, db.DB(), "sentinelcmd", 0)
			assertToolFTSQueryable(t, db.DB(), "sentinelcmd", len(want.ToolSearchItems))
			assertCurrentShape(t, db.DB())
			wantMigrationRows := authoritativeCurrentMigrationRows()
			if fixture.name == "v13-development-alter-built.sql" {
				for i := range wantMigrationRows {
					if wantMigrationRows[i].Version == 9 {
						wantMigrationRows[i].Checksum = "a84857185adb25a812c78f05b1ba4ee4d28e52b52b28812e077c03e7ae539917"
					}
				}
			}
			if got := loadStorageMigrationRows(t, db.DB()); !reflect.DeepEqual(got, wantMigrationRows) {
				t.Fatalf("schema_migrations rows differ\ngot:  %+v\nwant: %+v", got, wantMigrationRows)
			}

			snapshotPath := maybeOnlySnapshot(t, dbPath)
			if fixture.expectSnapshot && snapshotPath == "" {
				t.Fatalf("expected snapshot for fixture %s with destructive migration steps", fixture.name)
			}
			if !fixture.expectSnapshot && snapshotPath != "" {
				t.Fatalf("unexpected snapshot for fixture %s: %s", fixture.name, snapshotPath)
			}
			if snapshotPath != "" {
				snapshot, err := OpenReadOnly(snapshotPath)
				if err != nil {
					t.Fatalf("open snapshot read-only: %v", err)
				}
				defer func() { _ = snapshot.Close() }()
				assertSearchItems(t, snapshot.DB(), want.SearchItems)
				if fixture.appliedVersion < 5 {
					assertTableExists(t, snapshot.DB(), "session_events", true)
				}
				if fixture.hasSourceMetadata {
					assertColumnExists(t, snapshot.DB(), "search_items", "source_metadata", true)
				}
			}
		})
	}
}

func TestHistoricalLineageWithoutSourceMetadataUpgradesLosslessly(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v3-no-source-metadata.sql")
	wantRows := seedMigrationSentinels(t, dbPath)

	db, diag, err := OpenCompatible(context.Background(), dbPath)
	if err != nil || diag != nil {
		t.Fatalf("open compatible error=%v diagnostic=%+v", err, diag)
	}
	defer func() { _ = db.Close() }()

	assertSearchItems(t, db.DB(), wantRows)
	assertChunkCount(t, db.DB(), 1)
	assertFTSQueryable(t, db.DB(), "sentinelterm", len(wantRows))
	assertCurrentShape(t, db.DB())
}

func TestMigrationSnapshotAndRollbackOnDestructiveFailure(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v3-no-source-metadata.sql")
	wantRows := seedMigrationSentinels(t, dbPath)

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatalf("open without setup: %v", err)
	}
	defer func() { _ = db.Close() }()

	inspect, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open inspect: %v", err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}
	plan.Steps = []compat.MigrationStep{
		{Version: 4, Name: "V4 tool_fts trigram index"},
		{Version: 5, Name: "V5 drop phantom session_events"},
		{Version: 6, Name: "V6 drop source_metadata when present"},
	}
	if err := db.ApplyMigrationPlan(ctx, plan); err == nil {
		t.Fatal("expected destructive migration plan to fail on missing source_metadata")
	}

	assertSearchItems(t, db.DB(), wantRows)
	assertTableExists(t, db.DB(), "session_events", true)
	assertTableExists(t, db.DB(), "tool_fts", false)
	assertMigrationVersionCount(t, db.DB(), 4, 0)

	snapshotPath := onlySnapshot(t, dbPath)
	snapshot, err := OpenReadOnly(snapshotPath)
	if err != nil {
		t.Fatalf("open snapshot read-only: %v", err)
	}
	defer func() { _ = snapshot.Close() }()
	assertSearchItems(t, snapshot.DB(), wantRows)
	assertTableExists(t, snapshot.DB(), "session_events", true)
}

func TestOpenCompatibleClosesAndClearsDatabaseOnMigrationError(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v3.sql")
	migrationErr := fmt.Errorf("injected migration failure")
	originalApply := openCompatibleApplyMigrationPlan
	openCompatibleApplyMigrationPlan = func(_ *Database, _ context.Context, _ compat.MigrationPlan) error {
		return migrationErr
	}
	t.Cleanup(func() { openCompatibleApplyMigrationPlan = originalApply })

	db, diag, err := OpenCompatible(context.Background(), dbPath)
	if err == nil {
		t.Fatal("expected migration error")
	}
	if err != migrationErr {
		t.Fatalf("error = %v, want injected migration error", err)
	}
	if diag != nil {
		t.Fatalf("diagnostic = %+v, want nil", diag)
	}
	if db != nil {
		if pingErr := db.DB().Ping(); pingErr == nil {
			_ = db.Close()
			t.Fatal("OpenCompatible returned an open database after migration error")
		}
		t.Fatalf("db = %+v, want nil after migration error", db)
	}
}

func TestOpenCompatibleCreatesMissingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "new.db")
	db, diag, err := OpenCompatible(context.Background(), dbPath)
	if err != nil || diag != nil {
		t.Fatalf("open compatible missing error=%v diagnostic=%+v", err, diag)
	}
	defer func() { _ = db.Close() }()
	assertCurrentShape(t, db.DB())
}

func TestOpenUsesOrdinaryDeferredTransactions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ordinary.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	assertDatabaseBeginLeavesWriteReservationAvailable(t, dbPath, db.DB())
}

func TestOpenCompatibleReturnsOrdinaryHandleWithoutMigration(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v13.sql")
	db, diag, err := OpenCompatible(context.Background(), dbPath)
	if err != nil || diag != nil {
		t.Fatalf("open compatible error=%v diagnostic=%+v", err, diag)
	}
	defer func() { _ = db.Close() }()
	assertDatabaseBeginLeavesWriteReservationAvailable(t, dbPath, db.DB())
}

func TestOpenCompatibleReturnsOrdinaryHandleAfterMigration(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v7.sql")
	db, diag, err := OpenCompatible(context.Background(), dbPath)
	if err != nil || diag != nil {
		t.Fatalf("open compatible error=%v diagnostic=%+v", err, diag)
	}
	defer func() { _ = db.Close() }()
	assertDatabaseBeginLeavesWriteReservationAvailable(t, dbPath, db.DB())
}

func TestOpenCompatibleMigrationTransactionReservesWriteLock(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v7.sql")
	originalBegin := beginMigrationTx
	var checked bool
	beginMigrationTx = func(ctx context.Context, raw *sql.DB) (*sql.Tx, error) {
		tx, err := originalBegin(ctx, raw)
		if err != nil {
			return nil, err
		}
		checked = true
		assertCompetingBeginImmediateBlocked(t, dbPath)
		return tx, nil
	}
	t.Cleanup(func() { beginMigrationTx = originalBegin })

	db, diag, err := OpenCompatible(context.Background(), dbPath)
	if err != nil || diag != nil {
		t.Fatalf("open compatible error=%v diagnostic=%+v", err, diag)
	}
	defer func() { _ = db.Close() }()
	if !checked {
		t.Fatal("migration transaction was not observed")
	}
}

func TestSnapshotDatabaseUsesAvailableSiblingName(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v13.sql")
	if err := os.WriteFile(dbPath+".snapshot", []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshotPath, err := SnapshotDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("snapshot database: %v", err)
	}
	if snapshotPath != dbPath+".snapshot.1" {
		t.Fatalf("snapshot path = %q, want numbered sibling", snapshotPath)
	}
	snapshot, err := OpenReadOnly(snapshotPath)
	if err != nil {
		t.Fatalf("open snapshot read-only: %v", err)
	}
	defer func() { _ = snapshot.Close() }()
	assertCurrentShape(t, snapshot.DB())
}

func TestApplyMigrationPlanRejectsUnknownStep(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v13.sql")
	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatalf("open without setup: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.ApplyMigrationPlan(context.Background(), compat.MigrationPlan{Steps: []compat.MigrationStep{{Version: 99, Name: "unknown"}}}); err == nil {
		t.Fatal("expected unknown migration step to fail")
	}
	if err := db.ApplyMigrationPlan(context.Background(), compat.MigrationPlan{}); err != nil {
		t.Fatalf("empty migration plan: %v", err)
	}
}

func TestApplyMigrationPlanFromEmptySchemaCreatesCurrentShape(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "backscroll.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_on TEXT NOT NULL,
			checksum TEXT NOT NULL
		)
	`); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	_ = raw.Close()

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatalf("open without setup: %v", err)
	}
	defer func() { _ = db.Close() }()

	plan, _, err := compat.InspectIndex(ctx, db.DB())
	if err != nil {
		t.Fatalf("inspect empty schema: %v", err)
	}
	plan.Steps = []compat.MigrationStep{
		{Version: 1, Name: "V1 core schema"},
		{Version: 2, Name: "V2 embedding tables"},
		{Version: 3, Name: "V3 embedding blob column"},
		{Version: 4, Name: "V4 tool_fts trigram index"},
		{Version: 5, Name: "V5 drop phantom session_events"},
		{Version: 6, Name: "V6 drop source_metadata when present"},
		{Version: 7, Name: "V7 reasoning triggers"},
		{Version: 8, Name: "V8 perennity: extraction_version, was_interrupted, tool_events"},
		{Version: 9, Name: "V9 tool_events uuid uniqueness index"},
		{Version: 10, Name: "V10 template mining: message_templates, template_matches"},
		{Version: 11, Name: "V11 correction detection: correction_signals"},
		{Version: 12, Name: "V12 agent classification: annotations"},
		{Version: 13, Name: "V13 backfill discovery indexes"},
	}
	if err := db.ApplyMigrationPlan(ctx, plan); err != nil {
		t.Fatalf("apply full plan: %v", err)
	}
	assertCurrentShape(t, db.DB())
}

func TestV9HistoricalMigrationDefinitionIsImmutable(t *testing.T) {
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(sqlV9)))
	if checksum != "b16094805a4e08f6e0dd56bce5266c7c5fd71934389da9d13c4132076e546ca2" {
		t.Fatalf("sqlV9 checksum = %s, want published historical checksum", checksum)
	}
	for _, fixture := range []string{"v9.sql", "v10.sql", "v11.sql", "v12.sql", "v13.sql"} {
		data, err := os.ReadFile(filepath.Join("..", "compat", "testdata", "release-schemas", fixture))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "'b16094805a4e08f6e0dd56bce5266c7c5fd71934389da9d13c4132076e546ca2'") {
			t.Fatalf("%s does not preserve the published V9 ledger checksum", fixture)
		}
		if strings.Contains(string(data), "'a84857185adb25a812c78f05b1ba4ee4d28e52b52b28812e077c03e7ae539917'") {
			t.Fatalf("%s contains the replacement V9 checksum", fixture)
		}
	}
}

func TestSetupSchemaMigrationLedgerMatchesAuthoritativeDefinitions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "backscroll.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	got := loadStorageMigrationRows(t, db.DB())
	want := authoritativeCurrentMigrationRows()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema_migrations rows differ\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestMigrationFinalShapeFailureRollsBack(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v3.sql")
	wantRows := seedMigrationSentinels(t, dbPath)

	inspect, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open inspect: %v", err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}
	if len(plan.Steps) < 2 {
		t.Fatalf("plan too short: %+v", plan.Steps)
	}
	plan.Steps = plan.Steps[:len(plan.Steps)-1]

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatalf("open without setup: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.ApplyMigrationPlan(ctx, plan); err == nil {
		t.Fatal("expected final shape verification failure")
	}

	assertSearchItems(t, db.DB(), wantRows)
	assertMigrationVersionCount(t, db.DB(), 4, 0)
	assertTableExists(t, db.DB(), "tool_fts", false)
}

func TestDestructiveV8AndV9EntryPointsCreateSnapshot(t *testing.T) {
	for _, fixture := range []string{"v7.sql", "v8.sql"} {
		t.Run(fixture, func(t *testing.T) {
			dbPath := createFixtureDatabase(t, fixture)
			if fixture == "v8.sql" {
				seedToolEvent(t, dbPath, "v8-snapshot-uuid", "/v8-snapshot.jsonl", 1, "Bash", "echo ok", 0, 0, 8)
			}
			db, diag, err := OpenCompatible(context.Background(), dbPath)
			if err != nil || diag != nil {
				t.Fatalf("open compatible error=%v diagnostic=%+v", err, diag)
			}
			defer func() { _ = db.Close() }()
			if snapshotPath := maybeOnlySnapshot(t, dbPath); snapshotPath == "" {
				t.Fatalf("expected snapshot before destructive migration from %s", fixture)
			}
		})
	}
}

func TestApplyMigrationPlanSnapshotsBeforeBeginningTransaction(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v7.sql")
	inspect, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	originalSnapshot := snapshotDatabase
	originalBegin := beginMigrationTx
	var order []string
	snapshotDatabase = func(context.Context, string) (string, error) {
		order = append(order, "snapshot")
		return dbPath + ".snapshot", nil
	}
	beginMigrationTx = func(ctx context.Context, raw *sql.DB) (*sql.Tx, error) {
		order = append(order, "begin")
		return raw.BeginTx(ctx, nil)
	}
	t.Cleanup(func() {
		snapshotDatabase = originalSnapshot
		beginMigrationTx = originalBegin
	})

	if err := db.ApplyMigrationPlan(ctx, plan); err != nil {
		t.Fatalf("apply migration plan: %v", err)
	}
	if !reflect.DeepEqual(order[:2], []string{"snapshot", "begin"}) {
		t.Fatalf("snapshot/transaction order = %+v, want snapshot before begin", order)
	}
}

func TestV9DuplicateToolEventsConflictAbortsBeforeMutation(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v8.sql")
	seedToolEvent(t, dbPath, "dup-tool-uuid", "/dup-a.jsonl", 1, "Bash", "echo one", 0, 0, 8)
	seedToolEvent(t, dbPath, "dup-tool-uuid", "/dup-b.jsonl", 2, "Bash", "echo two", 0, 0, 8)

	inspect, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var snapshotCalled, beginCalled bool
	originalSnapshot := snapshotDatabase
	originalBegin := beginMigrationTx
	snapshotDatabase = func(ctx context.Context, path string) (string, error) {
		snapshotCalled = true
		return originalSnapshot(ctx, path)
	}
	beginMigrationTx = func(ctx context.Context, raw *sql.DB) (*sql.Tx, error) {
		beginCalled = true
		return raw.BeginTx(ctx, nil)
	}
	t.Cleanup(func() {
		snapshotDatabase = originalSnapshot
		beginMigrationTx = originalBegin
	})

	err = db.ApplyMigrationPlan(ctx, plan)
	if err == nil {
		t.Fatal("expected conflicting duplicate tool_events to abort V9")
	}
	if !strings.Contains(err.Error(), "conflicting tool_events") {
		t.Fatalf("error = %v, want conflicting tool_events", err)
	}
	if snapshotCalled || beginCalled {
		t.Fatalf("V9 duplicate conflict reached snapshot=%v begin=%v; want preflight abort before both", snapshotCalled, beginCalled)
	}
	assertToolEventUUIDCount(t, db.DB(), "dup-tool-uuid", 2)
	assertMigrationVersionCount(t, db.DB(), 9, 0)
	if snapshotPath := maybeOnlySnapshot(t, dbPath); snapshotPath != "" {
		t.Fatalf("unexpected snapshot for V9 preflight failure: %s", snapshotPath)
	}
}

func TestV9DuplicateToolEventsDifferingOnlyProvenanceAbortBeforeSnapshotOrTransaction(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v8.sql")
	seedToolEvent(t, dbPath, "provenance-tool-uuid", "/dup-a.jsonl", 1, "Bash", "echo same", 0, 0, 8)
	seedToolEvent(t, dbPath, "provenance-tool-uuid", "/dup-b.jsonl", 2, "Bash", "echo same", 0, 0, 8)

	inspect, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var snapshotCalled, beginCalled bool
	originalSnapshot := snapshotDatabase
	originalBegin := beginMigrationTx
	snapshotDatabase = func(ctx context.Context, path string) (string, error) {
		snapshotCalled = true
		return originalSnapshot(ctx, path)
	}
	beginMigrationTx = func(ctx context.Context, raw *sql.DB) (*sql.Tx, error) {
		beginCalled = true
		return raw.BeginTx(ctx, nil)
	}
	t.Cleanup(func() {
		snapshotDatabase = originalSnapshot
		beginMigrationTx = originalBegin
	})

	err = db.ApplyMigrationPlan(ctx, plan)
	if err == nil {
		t.Fatal("expected provenance-different duplicate tool_events to abort V9")
	}
	if !strings.Contains(err.Error(), "conflicting tool_events") {
		t.Fatalf("error = %v, want conflicting tool_events", err)
	}
	if snapshotCalled || beginCalled {
		t.Fatalf("V9 provenance conflict reached snapshot=%v begin=%v; want preflight abort before both", snapshotCalled, beginCalled)
	}
	assertToolEventUUIDCount(t, db.DB(), "provenance-tool-uuid", 2)
	assertMigrationVersionCount(t, db.DB(), 9, 0)
	if snapshotPath := maybeOnlySnapshot(t, dbPath); snapshotPath != "" {
		t.Fatalf("unexpected snapshot for V9 preflight failure: %s", snapshotPath)
	}
}

func TestV9ExactDuplicatePreflightIsReadOnlyAndAllowsMigrationSQLToCollapse(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE search_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL DEFAULT 'session',
			source_path TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			role TEXT NOT NULL,
			text TEXT NOT NULL,
			timestamp TEXT,
			uuid TEXT UNIQUE,
			project TEXT,
			content_type TEXT NOT NULL DEFAULT 'text',
			extraction_version INTEGER,
			was_interrupted INTEGER
		);
		CREATE TABLE tool_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_uuid TEXT,
			source_path TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			tool_name TEXT NOT NULL,
			command_head TEXT,
			is_error INTEGER,
			exit_code INTEGER,
			extraction_version INTEGER NOT NULL
		);
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/same.jsonl', 7, 'assistant', 'tool payload', '2026-08-18T00:00:00Z', 'exact-tool-uuid', 'project-a', 'tool', 8, 0);
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
		VALUES ('exact-tool-uuid', '/same.jsonl', 7, 'Bash', 'echo same', 0, 0, 8),
		       ('exact-tool-uuid', '/same.jsonl', 7, 'Bash', 'echo same', 0, 0, 8);
	`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := prepareV9ToolEventDuplicates(ctx, tx); err != nil {
		t.Fatalf("exact duplicate preflight rejected rows: %v", err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_events WHERE message_uuid = 'exact-tool-uuid'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("preflight changed exact duplicate count to %d, want 2", count)
	}
	if _, err := tx.ExecContext(ctx, sqlV9); err != nil {
		t.Fatalf("historical V9 SQL should collapse exact duplicates and create uniqueness index: %v", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tool_events WHERE message_uuid = 'exact-tool-uuid'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("historical V9 SQL retained %d exact duplicates, want 1", count)
	}
}

func TestV9StructuralDuplicateComparisonRejectsEmbeddedNULSerializationCollision(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE search_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL DEFAULT 'session',
			source_path TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			role TEXT NOT NULL,
			text TEXT NOT NULL,
			timestamp TEXT,
			uuid TEXT UNIQUE,
			project TEXT,
			content_type TEXT NOT NULL DEFAULT 'text',
			extraction_version INTEGER,
			was_interrupted INTEGER
		);
		CREATE TABLE tool_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_uuid TEXT,
			source_path TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			tool_name TEXT NOT NULL,
			command_head TEXT,
			is_error INTEGER,
			exit_code INTEGER,
			extraction_version INTEGER NOT NULL
		);
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
		VALUES ('nul-collision-tool-uuid', '/same.jsonl', 7, 'tool', 'cmd' || char(0) || '<null>', NULL, 0, 8),
		       ('nul-collision-tool-uuid', '/same.jsonl', 7, 'tool' || char(0) || 's:cmd', NULL, NULL, 0, 8);
	`); err != nil {
		t.Fatal(err)
	}

	err = prepareV9ToolEventDuplicates(ctx, db)
	if err == nil {
		t.Fatal("expected embedded-NUL structural difference to abort V9 preflight")
	}
	if !strings.Contains(err.Error(), "conflicting tool_events") {
		t.Fatalf("error = %v, want conflicting tool_events", err)
	}
}

func TestV9StructuralDuplicateComparisonPreservesNullableDistinctions(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE tool_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_uuid TEXT,
			source_path TEXT NOT NULL,
			ordinal INTEGER NOT NULL,
			tool_name TEXT NOT NULL,
			command_head TEXT,
			is_error INTEGER,
			exit_code INTEGER,
			extraction_version INTEGER NOT NULL
		);
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
		VALUES ('nullable-tool-uuid', '/same.jsonl', 7, 'Bash', '', NULL, NULL, 8),
		       ('nullable-tool-uuid', '/same.jsonl', 7, 'Bash', NULL, 0, 0, 8);
	`); err != nil {
		t.Fatal(err)
	}

	err = prepareV9ToolEventDuplicates(ctx, db)
	if err == nil {
		t.Fatal("expected NULL-vs-empty/zero structural differences to abort V9 preflight")
	}
	if !strings.Contains(err.Error(), "conflicting tool_events") {
		t.Fatalf("error = %v, want conflicting tool_events", err)
	}
}

func TestV9TransactionalRecheckCatchesDuplicateInsertedAfterPreflight(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v8.sql")
	seedToolEvent(t, dbPath, "late-duplicate-tool-uuid", "/late-a.jsonl", 1, "Bash", "echo one", 0, 0, 8)

	inspect, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	originalSnapshot := snapshotDatabase
	var injected bool
	snapshotDatabase = func(ctx context.Context, path string) (string, error) {
		seedToolEvent(t, path, "late-duplicate-tool-uuid", "/late-b.jsonl", 2, "Bash", "echo two", 0, 0, 8)
		injected = true
		return originalSnapshot(ctx, path)
	}
	t.Cleanup(func() { snapshotDatabase = originalSnapshot })

	err = db.ApplyMigrationPlan(ctx, plan)
	if err == nil {
		t.Fatal("expected transactional V9 recheck to abort late duplicate")
	}
	if !injected {
		t.Fatal("late duplicate was not injected after preflight")
	}
	if !strings.Contains(err.Error(), "conflicting tool_events") {
		t.Fatalf("error = %v, want conflicting tool_events", err)
	}
	raw, openErr := openWithoutSetup(dbPath)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer func() { _ = raw.Close() }()
	assertToolEventUUIDCount(t, raw.DB(), "late-duplicate-tool-uuid", 2)
	assertMigrationVersionCount(t, raw.DB(), 9, 0)
	assertIndexExists(t, raw.DB(), "idx_tool_events_uuid_unique", false)
}

func TestV9SamePayloadDifferentSourceIsNotAnExactDuplicate(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v8.sql")
	seedToolEvent(t, dbPath, "same-payload-tool-uuid", "/exact-a.jsonl", 1, "Bash", "echo same", 0, 0, 8)
	seedToolEvent(t, dbPath, "same-payload-tool-uuid", "/exact-b.jsonl", 2, "Bash", "echo same", 0, 0, 8)

	db, diag, err := OpenCompatible(ctx, dbPath)
	if err == nil {
		if db != nil {
			_ = db.Close()
		}
		t.Fatal("expected same-payload different-source duplicate tool_events to abort V9")
	}
	if diag != nil {
		t.Fatalf("diagnostic = %+v, want migration error", diag)
	}
	if !strings.Contains(err.Error(), "conflicting tool_events") {
		t.Fatalf("error = %v, want conflicting tool_events", err)
	}
	raw, openErr := openWithoutSetup(dbPath)
	if openErr != nil {
		t.Fatal(openErr)
	}
	defer func() { _ = raw.Close() }()
	assertToolEventUUIDCount(t, raw.DB(), "same-payload-tool-uuid", 2)
	if snapshotPath := maybeOnlySnapshot(t, dbPath); snapshotPath != "" {
		t.Fatalf("unexpected snapshot for V9 preflight failure: %s", snapshotPath)
	}
}

func TestApplyMigrationPlanUsesReceiverPathForSnapshotAndVerification(t *testing.T) {
	ctx := context.Background()
	receiverPath := createFixtureDatabase(t, "v3.sql")
	unrelatedPath := createFixtureDatabase(t, "v13.sql")

	inspect, err := OpenReadOnly(receiverPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}
	db, err := openWithoutSetup(receiverPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if err := db.ApplyMigrationPlan(ctx, plan); err != nil {
		t.Fatalf("apply migration plan: %v", err)
	}
	if snapshotPath := maybeOnlySnapshot(t, receiverPath); snapshotPath == "" {
		t.Fatal("receiver database was not snapshotted")
	}
	if snapshotPath := maybeOnlySnapshot(t, unrelatedPath); snapshotPath != "" {
		t.Fatalf("unrelated path was snapshotted: %s", snapshotPath)
	}
	assertCurrentShape(t, db.DB())
}

func TestApplyMigrationPlanRechecksShapeInsideTransaction(t *testing.T) {
	ctx := context.Background()
	dbPath := createFixtureDatabase(t, "v7.sql")
	inspect, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	_ = inspect.Close()
	if err != nil || diag != nil {
		t.Fatalf("inspect error=%v diagnostic=%+v", err, diag)
	}
	mutateFixtureDB(t, dbPath, `CREATE TABLE shape_changed_after_planning (id INTEGER PRIMARY KEY);`)

	db, err := openWithoutSetup(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	err = db.ApplyMigrationPlan(ctx, plan)
	if err == nil {
		t.Fatal("expected shape mismatch to abort")
	}
	if !strings.Contains(err.Error(), "changed since inspection") {
		t.Fatalf("error = %v, want changed since inspection", err)
	}
	assertMigrationVersionCount(t, db.DB(), 8, 0)
	assertTableExists(t, db.DB(), "shape_changed_after_planning", true)
}

type storageMigrationRow struct {
	Version  int
	Name     string
	Checksum string
}

type sentinelSearchItem struct {
	ID          int64
	Source      string
	SourcePath  string
	Ordinal     int
	Role        string
	Text        string
	Timestamp   string
	UUID        string
	Project     string
	ContentType string
}

func assertDatabaseBeginLeavesWriteReservationAvailable(t *testing.T, dbPath string, db *sql.DB) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin ordinary transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	competing, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatalf("open competing connection: %v", err)
	}
	defer func() { _ = competing.Close() }()
	if _, err := competing.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("ordinary BEGIN reserved the write lock; competing BEGIN IMMEDIATE failed: %v", err)
	}
	if _, err := competing.Exec("ROLLBACK"); err != nil {
		t.Fatalf("rollback competing transaction: %v", err)
	}
}

func assertCompetingBeginImmediateBlocked(t *testing.T, dbPath string) {
	t.Helper()
	competing, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatalf("open competing connection: %v", err)
	}
	defer func() { _ = competing.Close() }()
	if _, err := competing.Exec("BEGIN IMMEDIATE"); err == nil {
		_, _ = competing.Exec("ROLLBACK")
		t.Fatal("migration transaction did not reserve the write lock; competing BEGIN IMMEDIATE succeeded")
	}
}

func createFixtureDatabase(t *testing.T, fixture string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "compat", "testdata", "release-schemas", fixture))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	fixtureCopy := filepath.Join(tmp, fixture)
	if err := os.WriteFile(fixtureCopy, data, 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(tmp, "backscroll.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fixtureSQL, err := os.ReadFile(fixtureCopy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(fixtureSQL)); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func seedMigrationSentinels(t *testing.T, dbPath string) []sentinelSearchItem {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	items := []sentinelSearchItem{
		{Source: "session", SourcePath: "/project/session.jsonl", Ordinal: 1, Role: "user", Text: "sentinelterm first message", Timestamp: "2026-08-18T00:00:00Z", UUID: "sentinel-uuid-1", Project: "project", ContentType: "text"},
		{Source: "session", SourcePath: "/project/session.jsonl", Ordinal: 2, Role: "assistant", Text: "sentinelterm second message", Timestamp: "2026-08-18T00:00:00Z", UUID: "sentinel-uuid-2", Project: "project", ContentType: "text"},
	}
	for i := range items {
		res, err := db.Exec(`INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			items[i].Source, items[i].SourcePath, items[i].Ordinal, items[i].Role, items[i].Text, items[i].Timestamp, items[i].UUID, items[i].Project, items[i].ContentType)
		if err != nil {
			t.Fatal(err)
		}
		items[i].ID, err = res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
	}
	if tableExists(t, db, "chunks") {
		if columnExists(t, db, "chunks", "embedding") {
			if _, err := db.Exec(`INSERT INTO chunks (source_id, chunk_idx, content, token_count, created_at, embedding)
				VALUES ('sentinel-uuid-1', 0, 'chunk sentinel', 2, 1, X'0102')`); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := db.Exec(`INSERT INTO chunks (source_id, chunk_idx, content, token_count, created_at)
				VALUES ('sentinel-uuid-1', 0, 'chunk sentinel', 2, 1)`); err != nil {
				t.Fatal(err)
			}
		}
	}
	return items
}

func assertSearchItems(t *testing.T, db *sql.DB, want []sentinelSearchItem) {
	t.Helper()

	rows, err := db.Query(`SELECT id, source, source_path, ordinal, role, text, timestamp, uuid, project, content_type
		FROM search_items WHERE uuid LIKE 'sentinel-uuid-%' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []sentinelSearchItem
	for rows.Next() {
		var item sentinelSearchItem
		if err := rows.Scan(&item.ID, &item.Source, &item.SourcePath, &item.Ordinal, &item.Role, &item.Text, &item.Timestamp, &item.UUID, &item.Project, &item.ContentType); err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("search_items mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func assertSearchItemsByUUID(t *testing.T, db *sql.DB, want []sentinelSearchItem) {
	t.Helper()
	if len(want) == 0 {
		return
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(want)), ",")
	args := make([]any, 0, len(want))
	for _, item := range want {
		args = append(args, item.UUID)
	}
	rows, err := db.Query(`SELECT id, source, source_path, ordinal, role, text, timestamp, uuid, project, content_type
		FROM search_items WHERE uuid IN (`+placeholders+`) ORDER BY id`, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []sentinelSearchItem
	for rows.Next() {
		var item sentinelSearchItem
		if err := rows.Scan(&item.ID, &item.Source, &item.SourcePath, &item.Ordinal, &item.Role, &item.Text, &item.Timestamp, &item.UUID, &item.Project, &item.ContentType); err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("search_items by uuid mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func assertChunkCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM chunks WHERE source_id = 'sentinel-uuid-1'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("chunk count = %d, want %d", got, want)
	}
}

func assertFTSQueryable(t *testing.T, db *sql.DB, term string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH ?`, term).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("messages_fts count = %d, want %d", got, want)
	}
}

func assertToolFTSQueryable(t *testing.T, db *sql.DB, term string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tool_fts WHERE tool_fts MATCH ?`, term).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("tool_fts count = %d, want %d", got, want)
	}
}

type fixtureSentinels struct {
	SearchItems       []sentinelSearchItem
	ToolSearchItems   []sentinelSearchItem
	TableRows         map[string][][]string
	IndexedFiles      bool
	SessionTags       bool
	DynamicStopwords  bool
	Chunks            bool
	EmbeddingMetadata bool
	ToolEvents        bool
	MessageTemplates  bool
	TemplateMatches   bool
	CorrectionSignals bool
	Annotations       bool
}

func seedFixtureSentinels(t *testing.T, dbPath string) fixtureSentinels {
	t.Helper()
	want := fixtureSentinels{}
	want.SearchItems = seedMigrationSentinels(t, dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if tableExists(t, db, "indexed_files") {
		if _, err := db.Exec(`INSERT INTO indexed_files (path, hash, last_indexed) VALUES ('/sentinel/indexed.jsonl', 'sentinel-hash', '2026-08-18T00:00:00Z')`); err != nil {
			t.Fatal(err)
		}
		want.IndexedFiles = true
	}
	if tableExists(t, db, "session_tags") {
		if _, err := db.Exec(`INSERT INTO session_tags (source_path, tag) VALUES ('/project/session.jsonl', 'sentinel-tag')`); err != nil {
			t.Fatal(err)
		}
		want.SessionTags = true
	}
	if tableExists(t, db, "dynamic_stopwords") {
		if _, err := db.Exec(`INSERT INTO dynamic_stopwords (term) VALUES ('sentinelstopword')`); err != nil {
			t.Fatal(err)
		}
		want.DynamicStopwords = true
	}
	var chunkID int64
	if tableExists(t, db, "chunks") {
		var res sql.Result
		var err error
		if columnExists(t, db, "chunks", "embedding") {
			res, err = db.Exec(`INSERT OR IGNORE INTO chunks (source_id, chunk_idx, content, token_count, created_at, embedding)
				VALUES ('sentinel-uuid-2', 0, 'chunk sentinel two', 3, 1, X'0304')`)
		} else {
			res, err = db.Exec(`INSERT OR IGNORE INTO chunks (source_id, chunk_idx, content, token_count, created_at)
				VALUES ('sentinel-uuid-2', 0, 'chunk sentinel two', 3, 1)`)
		}
		if err != nil {
			t.Fatal(err)
		}
		chunkID, err = res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		want.Chunks = true
	}
	if chunkID != 0 && tableExists(t, db, "embedding_metadata") {
		if _, err := db.Exec(`INSERT INTO embedding_metadata (chunk_id, model_name, model_version, dimensions, created_at)
			VALUES (?, 'sentinel-model', 'v1', 2, 1)`, chunkID); err != nil {
			t.Fatal(err)
		}
		want.EmbeddingMetadata = true
	}
	if tableExists(t, db, "search_items") && columnExists(t, db, "search_items", "content_type") {
		toolItem := sentinelSearchItem{Source: "session", SourcePath: "/project/tool-session.jsonl", Ordinal: 101, Role: "assistant", Text: "sentinelcmd tool text", Timestamp: "2026-08-18T00:00:00Z", UUID: "tool-fts-sentinel-uuid", Project: "project", ContentType: "tool"}
		res, err := db.Exec(`INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, toolItem.Source, toolItem.SourcePath, toolItem.Ordinal, toolItem.Role, toolItem.Text, toolItem.Timestamp, toolItem.UUID, toolItem.Project, toolItem.ContentType)
		if err != nil {
			t.Fatal(err)
		}
		toolItem.ID, err = res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		want.ToolSearchItems = append(want.ToolSearchItems, toolItem)
	}
	if tableExists(t, db, "tool_events") {
		seedToolEventOpen(t, db, "sentinel-tool-event-uuid", "/project/tool-session.jsonl", 101, "Bash", "sentinelcmd", 0, 0, 8)
		want.ToolEvents = true
	}
	var templateID int64
	if tableExists(t, db, "message_templates") {
		res, err := db.Exec(`INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
			VALUES ('sentinel-template-signature', 1, 'sentinel template text', 1, '2026-08-18T00:00:00Z', '2026-08-18T00:00:00Z')`)
		if err != nil {
			t.Fatal(err)
		}
		templateID, err = res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		want.MessageTemplates = true
	}
	if templateID != 0 && tableExists(t, db, "template_matches") {
		if _, err := db.Exec(`INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
			VALUES (?, 'sentinel-uuid-1', '/project/session.jsonl', 1)`, templateID); err != nil {
			t.Fatal(err)
		}
		want.TemplateMatches = true
	}
	if tableExists(t, db, "correction_signals") {
		if _, err := db.Exec(`INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES ('sentinel-uuid-1', '/project/session.jsonl', 1, 'sentinel-detector', 0.9, 1)`); err != nil {
			t.Fatal(err)
		}
		want.CorrectionSignals = true
	}
	if tableExists(t, db, "annotations") {
		if _, err := db.Exec(`INSERT INTO annotations (item_uuid, source_path, ordinal, kind, label, source, created_at)
			VALUES ('sentinel-uuid-1', '/project/session.jsonl', 1, 'sentinel-kind', 'sentinel-label', 'agent', '2026-08-18T00:00:00Z')`); err != nil {
			t.Fatal(err)
		}
		want.Annotations = true
	}
	want.TableRows = snapshotSentinelTableRows(t, db)
	return want
}

func assertTableSentinels(t *testing.T, db *sql.DB, want fixtureSentinels) {
	t.Helper()
	for table, wantRows := range want.TableRows {
		gotRows := querySentinelTableRows(t, db, table)
		if !reflect.DeepEqual(gotRows, wantRows) {
			t.Fatalf("sentinel rows for %s differ\ngot:  %+v\nwant: %+v", table, gotRows, wantRows)
		}
	}
}

func snapshotSentinelTableRows(t *testing.T, db *sql.DB) map[string][][]string {
	t.Helper()
	result := map[string][][]string{}
	for _, table := range []string{
		"indexed_files",
		"session_tags",
		"dynamic_stopwords",
		"chunks",
		"embedding_metadata",
		"tool_events",
		"message_templates",
		"template_matches",
		"correction_signals",
		"annotations",
	} {
		if !tableExists(t, db, table) {
			continue
		}
		rows := querySentinelTableRows(t, db, table)
		if len(rows) > 0 {
			result[table] = rows
		}
	}
	return result
}

func querySentinelTableRows(t *testing.T, db *sql.DB, table string) [][]string {
	t.Helper()
	chunksQuery := `SELECT CAST(id AS TEXT), source_id, CAST(chunk_idx AS TEXT), content, CAST(token_count AS TEXT), CAST(created_at AS TEXT), '<null>' AS embedding FROM chunks WHERE source_id IN ('sentinel-uuid-1', 'sentinel-uuid-2') ORDER BY id`
	if table == "chunks" && columnExists(t, db, "chunks", "embedding") {
		chunksQuery = `SELECT CAST(id AS TEXT), source_id, CAST(chunk_idx AS TEXT), content, CAST(token_count AS TEXT), CAST(created_at AS TEXT), CASE WHEN embedding IS NULL THEN '<null>' ELSE hex(embedding) END FROM chunks WHERE source_id IN ('sentinel-uuid-1', 'sentinel-uuid-2') ORDER BY id`
	}
	queries := map[string]string{
		"indexed_files":      `SELECT path, hash, last_indexed FROM indexed_files WHERE path = '/sentinel/indexed.jsonl' ORDER BY path`,
		"session_tags":       `SELECT source_path, tag FROM session_tags WHERE tag = 'sentinel-tag' ORDER BY source_path, tag`,
		"dynamic_stopwords":  `SELECT term FROM dynamic_stopwords WHERE term = 'sentinelstopword' ORDER BY term`,
		"chunks":             chunksQuery,
		"embedding_metadata": `SELECT CAST(id AS TEXT), CAST(chunk_id AS TEXT), model_name, model_version, CAST(dimensions AS TEXT), CAST(created_at AS TEXT) FROM embedding_metadata WHERE model_name = 'sentinel-model' ORDER BY id`,
		"tool_events":        `SELECT CAST(id AS TEXT), message_uuid, source_path, CAST(ordinal AS TEXT), tool_name, COALESCE(command_head, '<null>'), COALESCE(CAST(is_error AS TEXT), '<null>'), COALESCE(CAST(exit_code AS TEXT), '<null>'), CAST(extraction_version AS TEXT) FROM tool_events WHERE message_uuid = 'sentinel-tool-event-uuid' ORDER BY id`,
		"message_templates":  `SELECT CAST(id AS TEXT), signature, CAST(normalization_version AS TEXT), template_text, CAST(occurrence_count AS TEXT), COALESCE(first_seen, '<null>'), COALESCE(last_seen, '<null>') FROM message_templates WHERE signature = 'sentinel-template-signature' ORDER BY id`,
		"template_matches":   `SELECT CAST(id AS TEXT), CAST(template_id AS TEXT), COALESCE(item_uuid, '<null>'), source_path, CAST(ordinal AS TEXT) FROM template_matches WHERE item_uuid = 'sentinel-uuid-1' ORDER BY id`,
		"correction_signals": `SELECT CAST(id AS TEXT), COALESCE(item_uuid, '<null>'), source_path, CAST(ordinal AS TEXT), detector, CAST(confidence AS TEXT), CAST(extraction_version AS TEXT) FROM correction_signals WHERE detector = 'sentinel-detector' ORDER BY id`,
		"annotations":        `SELECT CAST(id AS TEXT), COALESCE(item_uuid, '<null>'), source_path, CAST(ordinal AS TEXT), kind, label, source, created_at FROM annotations WHERE kind = 'sentinel-kind' ORDER BY id`,
	}
	query, ok := queries[table]
	if !ok {
		t.Fatalf("no sentinel query for %s", table)
	}
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("query sentinel %s: %v", table, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][]string
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		row := make([]string, len(columns))
		for i, value := range values {
			if value.Valid {
				row[i] = value.String
			} else {
				row[i] = "<null>"
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func fixtureNeedsDestructiveSnapshot(appliedVersion int, hasSourceMetadata bool) bool {
	if appliedVersion < 5 || (appliedVersion < 6 && hasSourceMetadata) {
		return true
	}
	return appliedVersion < 9
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count == 1
}

func assertIndexExists(t *testing.T, db *sql.DB, index string, want bool) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if got := count == 1; got != want {
		t.Fatalf("index %s exists = %v, want %v", index, got, want)
	}
}

func assertColumnExists(t *testing.T, db *sql.DB, table, column string, want bool) {
	t.Helper()
	got := columnExists(t, db, table, column)
	if got != want {
		t.Fatalf("column %s.%s exists = %v, want %v", table, column, got, want)
	}
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + quoteIdentifierForTest(table) + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func maybeOnlySnapshot(t *testing.T, dbPath string) string {
	t.Helper()
	matches, err := filepath.Glob(dbPath + ".snapshot*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 1 {
		t.Fatalf("snapshot matches = %+v, want at most one", matches)
	}
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

func quoteIdentifierForTest(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func seedToolEvent(t *testing.T, dbPath, messageUUID, sourcePath string, ordinal int, toolName, commandHead string, isError, exitCode, extractionVersion int) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	return seedToolEventOpen(t, db, messageUUID, sourcePath, ordinal, toolName, commandHead, isError, exitCode, extractionVersion)
}

func seedToolEventOpen(t *testing.T, db *sql.DB, messageUUID, sourcePath string, ordinal int, toolName, commandHead string, isError, exitCode, extractionVersion int) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, messageUUID, sourcePath, ordinal, toolName, commandHead, isError, exitCode, extractionVersion)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertToolEventUUIDCount(t *testing.T, db *sql.DB, messageUUID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tool_events WHERE message_uuid = ?`, messageUUID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("tool_events count for %s = %d, want %d", messageUUID, got, want)
	}
}

func mutateFixtureDB(t *testing.T, dbPath string, stmt string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
}
func loadStorageMigrationRows(t *testing.T, db *sql.DB) []storageMigrationRow {
	t.Helper()

	rows, err := db.Query(`SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var result []storageMigrationRow
	for rows.Next() {
		var row storageMigrationRow
		if err := rows.Scan(&row.Version, &row.Name, &row.Checksum); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func authoritativeCurrentMigrationRows() []storageMigrationRow {
	return []storageMigrationRow{
		{Version: 1, Name: "V1 core schema", Checksum: "4e07949ccd3912fb3c0e149be9a2e05fdd51f8cedb8df1f28b3bb5ac5afe532a"},
		{Version: 2, Name: "V2 embedding tables", Checksum: "37dc9627f01f0e2d0fbea6bba5cd9f609d5da05089eeb9541a057dd2290cf8af"},
		{Version: 3, Name: "V3 embedding blob column", Checksum: "36cd183f10ff84ab4753be027078cdd710b46efdc830053d4a641af725006ea5"},
		{Version: 4, Name: "V4 tool_fts trigram index", Checksum: "77e59cf515f33c466282e0f3e921377f584794d0b7cade1704f566374a569a55"},
		{Version: 5, Name: "V5 drop phantom session_events", Checksum: "aedaab81efb6bc34d3f664468b71c1a716f5b55d5132156cb43bd7462d549c7b"},
		{Version: 6, Name: "V6 drop phantom source_metadata column", Checksum: "a327b9b6e7b8f5fe369c9fc08093ac80a87640daa515890af94b269d430e9378"},
		{Version: 7, Name: "V7 reasoning content_type routes to messages_fts", Checksum: "a80704442c2a0084f98e4bc53978364b14d1c6bd3bd99ba6119a2a9ecbd685e7"},
		{Version: 8, Name: "V8 perennity: extraction_version, was_interrupted, tool_events", Checksum: "6853d72ded3bdc775b52507321c31df44bf35e8277719432ca8aa126ee16cec1"},
		{Version: 9, Name: "V9 tool_events uuid uniqueness index", Checksum: "b16094805a4e08f6e0dd56bce5266c7c5fd71934389da9d13c4132076e546ca2"},
		{Version: 10, Name: "V10 template mining: message_templates, template_matches", Checksum: "0e548d0cb6c47147726f943bfc860500ca9a9bc821df0601998876ea5e9652c2"},
		{Version: 11, Name: "V11 correction detection: correction_signals", Checksum: "5a7180f901c5feacb67cd104a61e7b7ba9cec69aaeab1d2b5d723459dc590456"},
		{Version: 12, Name: "V12 agent classification: annotations (free-form labels; enum freeze deferred)", Checksum: "b3fb66fd2924a9f07a3e4ec0ba253fc1d6965664615a795be6b22d01ab292108"},
		{Version: 13, Name: "V13 backfill discovery indexes", Checksum: "2172ce531c670806933ffe3005fdc0a2ebb8eb3f84d2bd0d8fa608dddb5d136e"},
	}
}

func assertCurrentShape(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := compat.VerifyCurrentShape(context.Background(), db); err != nil {
		t.Fatalf("verify current shape: %v", err)
	}
}

func assertTableExists(t *testing.T, db *sql.DB, table string, want bool) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if got := count == 1; got != want {
		t.Fatalf("table %s exists = %v, want %v", table, got, want)
	}
}

func assertMigrationVersionCount(t *testing.T, db *sql.DB, version int, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("migration version %d count = %d, want %d", version, got, want)
	}
}

func onlySnapshot(t *testing.T, dbPath string) string {
	t.Helper()
	matches, err := filepath.Glob(dbPath + ".snapshot*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("snapshot matches = %+v, want exactly one", matches)
	}
	return matches[0]
}

// TestMigratedFixtureSignatureIsInCatalog is a regression test for issue #52.
// It verifies that when a V1 fixture is migrated forward through the real
// SetupSchema() code, the resulting database schema signature is recognized by
// the catalog. This prevents cosmetic DDL formatting differences from silently
// ejecting valid schemas from the lineage catalog.
//
// Before the fix to normalizeSQL() (making it whitespace-insensitive), databases
// that were created at V1 and then migrated V8→V13 by published releases would
// produce a signature not in the catalog, causing every operational command to
// reject the database as "unsupported_lineage". This test would have caught that
// regression during development.
func TestMigratedFixtureSignatureIsInCatalog(t *testing.T) {
	// Load v1.sql, the earliest released version fixture
	dbPath := createFixtureDatabase(t, "v1.sql")

	// Open the fixture and trigger SetupSchema to migrate it all the way to V13
	db, diag, err := OpenCompatible(context.Background(), dbPath)
	if err != nil || diag != nil {
		t.Fatalf("open compatible error=%v diagnostic=%+v", err, diag)
	}
	defer func() { _ = db.Close() }()

	// Inspect the resulting schema to get its signature
	plan, diag, err := compat.InspectIndex(context.Background(), db.DB())
	if err != nil || diag != nil {
		t.Fatalf("inspect index error=%v diagnostic=%+v", err, diag)
	}

	// Load the lineage catalog
	catalog, err := compat.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}

	// Assert that the migrated database signature is in the catalog
	if !catalog.IsKnownSignature(plan.From.Signature) {
		t.Errorf("migrated V1→V13 database has signature %s not in catalog; this was the bug in issue #52",
			plan.From.Signature)
	}
}
