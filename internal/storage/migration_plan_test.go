package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
)

func TestPublishedGoLineagesUpgradeLosslessly(t *testing.T) {
	dbPath := createFixtureDatabase(t, "v3.sql")
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

	plan := compat.MigrationPlan{
		From: compat.SchemaShape{AppliedVersion: 3},
		Steps: []compat.MigrationStep{
			{Version: 4, Name: "V4 tool_fts trigram index"},
			{Version: 5, Name: "V5 drop phantom session_events"},
			{Version: 6, Name: "V6 drop source_metadata when present"},
		},
	}
	if err := db.ApplyMigrationPlan(ctx, dbPath, plan); err == nil {
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
	openCompatibleApplyMigrationPlan = func(_ *Database, _ context.Context, _ string, _ compat.MigrationPlan) error {
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
	if err := db.ApplyMigrationPlan(context.Background(), dbPath, compat.MigrationPlan{Steps: []compat.MigrationStep{{Version: 99, Name: "unknown"}}}); err == nil {
		t.Fatal("expected unknown migration step to fail")
	}
	if err := db.ApplyMigrationPlan(context.Background(), dbPath, compat.MigrationPlan{}); err != nil {
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

	plan := compat.MigrationPlan{Steps: []compat.MigrationStep{
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
	}}
	if err := db.ApplyMigrationPlan(ctx, dbPath, plan); err != nil {
		t.Fatalf("apply full plan: %v", err)
	}
	assertCurrentShape(t, db.DB())
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

	if err := db.ApplyMigrationPlan(ctx, dbPath, plan); err == nil {
		t.Fatal("expected final shape verification failure")
	}

	assertSearchItems(t, db.DB(), wantRows)
	assertMigrationVersionCount(t, db.DB(), 4, 0)
	assertTableExists(t, db.DB(), "tool_fts", false)
}

type storageMigrationRow struct {
	Version  int
	Name     string
	Checksum string
}

type sentinelSearchItem struct {
	ID          int64
	SourcePath  string
	Ordinal     int
	Role        string
	Text        string
	UUID        string
	Project     string
	ContentType string
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
		{SourcePath: "/project/session.jsonl", Ordinal: 1, Role: "user", Text: "sentinelterm first message", UUID: "sentinel-uuid-1", Project: "project", ContentType: "text"},
		{SourcePath: "/project/session.jsonl", Ordinal: 2, Role: "assistant", Text: "sentinelterm second message", UUID: "sentinel-uuid-2", Project: "project", ContentType: "text"},
	}
	for i := range items {
		res, err := db.Exec(`INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES ('session', ?, ?, ?, ?, '2026-08-18T00:00:00Z', ?, ?, ?)`,
			items[i].SourcePath, items[i].Ordinal, items[i].Role, items[i].Text, items[i].UUID, items[i].Project, items[i].ContentType)
		if err != nil {
			t.Fatal(err)
		}
		items[i].ID, err = res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO chunks (source_id, chunk_idx, content, token_count, created_at, embedding)
		VALUES ('sentinel-uuid-1', 0, 'chunk sentinel', 2, 1, X'0102')`); err != nil {
		t.Fatal(err)
	}
	return items
}

func assertSearchItems(t *testing.T, db *sql.DB, want []sentinelSearchItem) {
	t.Helper()

	rows, err := db.Query(`SELECT id, source_path, ordinal, role, text, uuid, project, content_type
		FROM search_items WHERE uuid LIKE 'sentinel-uuid-%' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var got []sentinelSearchItem
	for rows.Next() {
		var item sentinelSearchItem
		if err := rows.Scan(&item.ID, &item.SourcePath, &item.Ordinal, &item.Role, &item.Text, &item.UUID, &item.Project, &item.ContentType); err != nil {
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
		t.Fatalf("FTS count = %d, want %d", got, want)
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
