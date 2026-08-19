package storage

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/models"
)

type recoveryFixtureHandle struct {
	db             *Database
	dbPath         string
	dbSnapshot     recoveryDBSnapshot
	sourcePath     string
	sourceSnapshot recoveryFileSnapshot
}

type recoveryFileSnapshot struct {
	data    []byte
	modTime time.Time
}

type recoveryDBSnapshot struct {
	main     recoveryFileSnapshot
	sidecars map[string]recoveryFileSnapshot
}

func TestReadRecoveryInputAdaptsSupportedLineages(t *testing.T) {
	for _, fixture := range []string{"active-v13.sql", "stranded-v3-no-source-metadata.sql", "stranded-v7.sql"} {
		t.Run(fixture, func(t *testing.T) {
			handle := openRecoveryFixtureReadOnly(t, fixture)
			defer func() { _ = handle.db.Close() }()

			got, diag, err := ReadRecoveryInput(context.Background(), handle.db)
			if err != nil || diag != nil {
				t.Fatalf("ReadRecoveryInput err=%v diagnostic=%+v", err, diag)
			}
			if got.RowCount != 2 || got.RowCount != len(got.Records) {
				t.Fatalf("input row count = %d records = %d, want 2", got.RowCount, len(got.Records))
			}
			if got.Shape.Signature == "" || got.Shape.AppliedVersion == 0 {
				t.Fatalf("shape was not populated from inspection: %+v", got.Shape)
			}
			assertRecoveryRecordsEqual(t, got.Records, expectedRecoveryRecords(fixture))
			assertQueryOnly(t, handle.db, true)
			assertRecoveryFixtureUnchanged(t, handle)
		})
	}
}

func TestReadRecoveryInputSupportsCatalogReadableSignatures(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join("..", "compat", "testdata", "release-schemas", "v*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no catalog schema fixtures found")
	}
	sort.Strings(fixtures)

	for _, fixturePath := range fixtures {
		fixture := filepath.Base(fixturePath)
		t.Run(fixture, func(t *testing.T) {
			dbPath := buildRecoveryDatabaseFromSQLFile(t, fixturePath)
			slug := strings.TrimSuffix(fixture, ".sql")
			insertRecoverySentinel(t, dbPath, slug)
			handle := openRecoveryDBReadOnly(t, dbPath, fixturePath)
			defer func() { _ = handle.db.Close() }()

			got, diag, err := ReadRecoveryInput(context.Background(), handle.db)
			if err != nil || diag != nil {
				t.Fatalf("ReadRecoveryInput err=%v diagnostic=%+v", err, diag)
			}
			if got.RowCount != 1 || len(got.Records) != 1 {
				t.Fatalf("row count = %d records = %d, want 1", got.RowCount, len(got.Records))
			}
			assertRecoveryRecordsEqual(t, got.Records, []models.IndexedRecord{expectedCatalogRecoveryRecord(slug)})
			assertQueryOnly(t, handle.db, true)
			assertRecoveryFixtureUnchanged(t, handle)
		})
	}
}

func TestReadRecoveryInputRejectsUnknownShape(t *testing.T) {
	dbPath := buildRecoveryFixtureDatabase(t, "active-v13.sql")
	mutateRecoveryDatabase(t, dbPath, `CREATE TABLE recovery_unknown_shape (id INTEGER PRIMARY KEY);`)
	handle := openRecoveryDBReadOnly(t, dbPath, recoveryFixturePath("active-v13.sql"))
	defer func() { _ = handle.db.Close() }()

	got, diag, err := ReadRecoveryInput(context.Background(), handle.db)
	if err != nil {
		t.Fatalf("ReadRecoveryInput error = %v", err)
	}
	if diag == nil || diag.Code != compat.CodeUnsupportedLineage {
		t.Fatalf("diagnostic = %+v, want CodeUnsupportedLineage", diag)
	}
	if len(diag.Continuation) != 0 {
		t.Fatalf("diagnostic continuation = %v, want empty", diag.Continuation)
	}
	if got.RowCount != 0 || len(got.Records) != 0 {
		t.Fatalf("input = %+v, want empty on unsupported lineage", got)
	}
	assertQueryOnly(t, handle.db, true)
	assertRecoveryFixtureUnchanged(t, handle)
}

func TestReadRecoveryInputRejectsMissingCanonicalPayload(t *testing.T) {
	dbPath := buildRecoveryFixtureDatabase(t, "active-v13.sql")
	mutateRecoveryDatabase(t, dbPath, `
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session', '/fixtures/recovery/corrupt.jsonl', 'not-an-integer', 'user', 'corrupt ordinal sentinel', 'text');
	`)
	handle := openRecoveryDBReadOnly(t, dbPath, recoveryFixturePath("active-v13.sql"))
	defer func() { _ = handle.db.Close() }()

	got, diag, err := ReadRecoveryInput(context.Background(), handle.db)
	if err != nil {
		t.Fatalf("ReadRecoveryInput error = %v", err)
	}
	if diag == nil || diag.Code != compat.CodeUninterpretableRow {
		t.Fatalf("diagnostic = %+v, want CodeUninterpretableRow", diag)
	}
	if len(diag.Continuation) != 0 {
		t.Fatalf("diagnostic continuation = %v, want empty", diag.Continuation)
	}
	if got.RowCount != 0 || len(got.Records) != 0 {
		t.Fatalf("input = %+v, want empty on corrupt row", got)
	}
	assertQueryOnly(t, handle.db, true)
	assertRecoveryFixtureUnchanged(t, handle)
}

func TestReadRecoveryInputPerformsNoWrites(t *testing.T) {
	for _, fixture := range []string{"active-v13.sql", "stranded-v3-no-source-metadata.sql", "stranded-v7.sql"} {
		t.Run(fixture, func(t *testing.T) {
			handle := openRecoveryFixtureReadOnly(t, fixture)
			defer func() { _ = handle.db.Close() }()

			if _, diag, err := ReadRecoveryInput(context.Background(), handle.db); err != nil || diag != nil {
				t.Fatalf("ReadRecoveryInput err=%v diagnostic=%+v", err, diag)
			}
			assertQueryOnly(t, handle.db, true)
			assertRecoveryFixtureUnchanged(t, handle)
		})
	}
}

func expectedRecoveryRecords(fixture string) []models.IndexedRecord {
	slug := strings.TrimSuffix(fixture, ".sql")
	project := "project-" + slug
	uuid := "uuid-" + slug
	timestamp := "2026-08-18T12:34:56Z"
	return []models.IndexedRecord{
		{
			Source:      "session",
			SourcePath:  "/fixtures/recovery/" + slug + "-defaults.jsonl",
			Ordinal:     42,
			Role:        "user",
			Text:        "default sentinel for " + slug,
			ContentType: "text",
		},
		{
			Source:      "source-" + slug,
			SourcePath:  "/fixtures/recovery/" + slug + ".jsonl",
			Ordinal:     41,
			Role:        "assistant",
			Text:        "text sentinel for " + slug,
			Project:     &project,
			UUID:        &uuid,
			Timestamp:   &timestamp,
			ContentType: "content/" + slug,
		},
	}
}

func expectedCatalogRecoveryRecord(slug string) models.IndexedRecord {
	project := "project-" + slug
	uuid := "uuid-" + slug
	timestamp := "2026-08-18T12:34:56Z"
	return models.IndexedRecord{
		Source:      "source-" + slug,
		SourcePath:  "/fixtures/recovery/" + slug + ".jsonl",
		Ordinal:     7,
		Role:        "assistant",
		Text:        "catalog sentinel for " + slug,
		Project:     &project,
		UUID:        &uuid,
		Timestamp:   &timestamp,
		ContentType: "content/" + slug,
	}
}

func assertRecoveryRecordsEqual(t *testing.T, got, want []models.IndexedRecord) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("records mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func openRecoveryFixtureReadOnly(t *testing.T, fixture string) recoveryFixtureHandle {
	t.Helper()
	sourcePath := recoveryFixturePath(fixture)
	dbPath := buildRecoveryFixtureDatabase(t, fixture)
	return openRecoveryDBReadOnly(t, dbPath, sourcePath)
}

func recoveryFixturePath(fixture string) string {
	return filepath.Join("..", "..", "tests", "fixtures", "recovery", fixture)
}

func buildRecoveryFixtureDatabase(t *testing.T, fixture string) string {
	t.Helper()
	return buildRecoveryDatabaseFromSQLFile(t, recoveryFixturePath(fixture))
}

func buildRecoveryDatabaseFromSQLFile(t *testing.T, fixturePath string) string {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "backscroll.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(string(data)); err != nil {
		t.Fatalf("load fixture %s: %v", fixturePath, err)
	}
	return dbPath
}

func insertRecoverySentinel(t *testing.T, dbPath, slug string) {
	t.Helper()
	mutateRecoveryDatabase(t, dbPath, fmt.Sprintf(`
		INSERT INTO indexed_files (path, hash, last_indexed)
		VALUES ('/fixtures/recovery/%[1]s.jsonl', 'hash-%[1]s', '2026-08-18T00:00:00Z');
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
		VALUES ('source-%[1]s', '/fixtures/recovery/%[1]s.jsonl', 7, 'assistant', 'catalog sentinel for %[1]s', '2026-08-18T12:34:56Z', 'uuid-%[1]s', 'project-%[1]s', 'content/%[1]s');
	`, slug))
}

func mutateRecoveryDatabase(t *testing.T, dbPath string, stmt string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
}

func openRecoveryDBReadOnly(t *testing.T, dbPath, sourcePath string) recoveryFixtureHandle {
	t.Helper()
	handle := recoveryFixtureHandle{
		dbPath:         dbPath,
		dbSnapshot:     snapshotRecoveryDB(t, dbPath),
		sourcePath:     sourcePath,
		sourceSnapshot: snapshotRecoveryFile(t, sourcePath),
	}
	db, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	db.DB().SetMaxOpenConns(1)
	if _, err := db.DB().Exec("PRAGMA query_only = ON"); err != nil {
		_ = db.Close()
		t.Fatalf("enable query_only: %v", err)
	}
	handle.db = db
	assertQueryOnly(t, handle.db, true)
	return handle
}

func assertQueryOnly(t *testing.T, db *Database, want bool) {
	t.Helper()
	var gotInt int
	if err := db.DB().QueryRow("PRAGMA query_only").Scan(&gotInt); err != nil {
		t.Fatalf("query PRAGMA query_only: %v", err)
	}
	got := gotInt != 0
	if got != want {
		t.Fatalf("PRAGMA query_only = %v, want %v", got, want)
	}
}

func assertRecoveryFixtureUnchanged(t *testing.T, handle recoveryFixtureHandle) {
	t.Helper()
	assertRecoveryFileSnapshot(t, "source SQL fixture", handle.sourcePath, handle.sourceSnapshot)
	assertRecoveryDBSnapshot(t, handle.dbPath, handle.dbSnapshot)
}

func assertRecoveryDBSnapshot(t *testing.T, dbPath string, want recoveryDBSnapshot) {
	t.Helper()
	assertRecoveryFileSnapshot(t, "database file", dbPath, want.main)
	got := snapshotRecoverySidecars(t, dbPath)
	if !reflect.DeepEqual(sortedMapKeys(got), sortedMapKeys(want.sidecars)) {
		t.Fatalf("sidecar inventory = %v, want %v", sortedMapKeys(got), sortedMapKeys(want.sidecars))
	}
	for name, wantSnapshot := range want.sidecars {
		gotSnapshot := got[name]
		if !bytes.Equal(gotSnapshot.data, wantSnapshot.data) || !gotSnapshot.modTime.Equal(wantSnapshot.modTime) {
			t.Fatalf("sidecar %s changed", name)
		}
	}
}

func snapshotRecoveryDB(t *testing.T, dbPath string) recoveryDBSnapshot {
	t.Helper()
	return recoveryDBSnapshot{
		main:     snapshotRecoveryFile(t, dbPath),
		sidecars: snapshotRecoverySidecars(t, dbPath),
	}
}

func snapshotRecoverySidecars(t *testing.T, dbPath string) map[string]recoveryFileSnapshot {
	t.Helper()
	dir := filepath.Dir(dbPath)
	base := filepath.Base(dbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshots := map[string]recoveryFileSnapshot{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, base+"-") {
			continue
		}
		snapshots[name] = snapshotRecoveryFile(t, filepath.Join(dir, name))
	}
	return snapshots
}

func assertRecoveryFileSnapshot(t *testing.T, label, path string, want recoveryFileSnapshot) {
	t.Helper()
	got := snapshotRecoveryFile(t, path)
	if !bytes.Equal(got.data, want.data) {
		t.Fatalf("%s bytes changed for %s", label, path)
	}
	if !got.modTime.Equal(want.modTime) {
		t.Fatalf("%s mtime changed for %s: got %s want %s", label, path, got.modTime, want.modTime)
	}
}

func snapshotRecoveryFile(t *testing.T, path string) recoveryFileSnapshot {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return recoveryFileSnapshot{data: data, modTime: info.ModTime()}
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
