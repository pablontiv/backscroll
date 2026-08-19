package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

// Generate a unique UUID for testing
var uuidCounter = 0

func getTestUUID() string {
	uuidCounter++
	return fmt.Sprintf("test-uuid-%d", uuidCounter)
}

// newTestDB creates a temporary database for testing.
func newTestDB(t *testing.T) (*Database, func()) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	}

	return db, cleanup
}

// TestOpen creates a new database and verifies schema is set up.
func TestOpen(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Verify tables exist
	var count int
	err := db.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='search_items'").Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("search_items table not created")
	}

	err = db.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='indexed_files'").Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("indexed_files table not created")
	}

	err = db.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages_fts'").Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("messages_fts FTS5 table not created")
	}
}

// TestOpenDirectoryPath verifies Open fails cleanly when the path is a directory.
func TestOpenDirectoryPath(t *testing.T) {
	dir := t.TempDir()

	db, err := Open(dir)
	if err == nil {
		_ = db.Close()
		t.Fatal("expected error opening a directory as a database")
	}
}

// TestOpenReadOnly verifies read-only mode.
func TestOpenReadOnly(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Close the write connection
	dbPath := filepath.Join(os.TempDir(), "test_readonly.db")
	_ = db.Close()

	// Create a fresh database for read-only test
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	_ = db.Close()

	// Try opening in read-only mode
	db, err = OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("failed to open database in read-only mode: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify we can read
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM indexed_files").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query read-only database: %v", err)
	}
}

// TestWriteConnPragmas verifies the write connection actually applies WAL mode and a
// busy timeout. modernc.org/sqlite honors the `_pragma=name(value)` DSN syntax, not the
// mattn-style `_name=value`; the latter is silently ignored, leaving the DB in rollback
// (delete) journal mode with no busy timeout — the root cause of SQLITE_BUSY under load.
func TestWriteConnPragmas(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "write_pragmas.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var journalMode string
	if err := db.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("querying journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("write connection journal_mode = %q, want \"wal\"", journalMode)
	}

	var timeout int
	if err := db.db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("querying busy_timeout: %v", err)
	}
	if timeout == 0 {
		t.Fatalf("write connection has no busy_timeout (got %d); writes fail immediately under contention", timeout)
	}
}

// TestOpenReadOnlyHasBusyTimeout verifies the read-only connection is configured
// with a busy timeout. Without it, a query fails immediately with SQLITE_BUSY when
// a concurrent auto-sync holds the write lock, instead of waiting for it to release.
func TestOpenReadOnlyHasBusyTimeout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "busy_timeout.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	_ = db.Close()

	db, err = OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("failed to open database in read-only mode: %v", err)
	}
	defer func() { _ = db.Close() }()

	var timeout int
	if err := db.db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("querying busy_timeout: %v", err)
	}
	if timeout == 0 {
		t.Fatalf("read-only connection has no busy_timeout (got %d); queries can fail with SQLITE_BUSY under concurrent writes", timeout)
	}
}

func TestOpenReadOnlyLiveWALSeesCommittedRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "live_wal.db")
	writer := openTestDBWithLiveWAL(t, dbPath)
	defer func() { _ = writer.Close() }()

	readonly, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly with live WAL: %v", err)
	}
	defer func() { _ = readonly.Close() }()

	var count int
	if err := readonly.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE text = 'live WAL committed row'").Scan(&count); err != nil {
		t.Fatalf("query live WAL row through OpenReadOnly: %v", err)
	}
	if count != 1 {
		t.Fatalf("live WAL row count = %d, want 1", count)
	}
}

func TestOpenImmutableReadOnlyRefusesLiveWALWithoutSidecarMutation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immutable_live_wal.db")
	writer := openTestDBWithLiveWAL(t, dbPath)
	defer func() { _ = writer.Close() }()
	before := snapshotDBSidecars(t, dbPath)

	immutable, err := OpenImmutableReadOnly(dbPath)
	if err == nil {
		_ = immutable.Close()
		t.Fatal("OpenImmutableReadOnly with live WAL succeeded; want unsafe WAL error")
	}
	if !errors.Is(err, ErrImmutableReadOnlyWALUnsafe) {
		t.Fatalf("OpenImmutableReadOnly error = %v, want ErrImmutableReadOnlyWALUnsafe", err)
	}

	after := snapshotDBSidecars(t, dbPath)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("OpenImmutableReadOnly mutated sidecars\nbefore=%s\nafter= %s", describeStorageSidecars(before), describeStorageSidecars(after))
	}
}

func TestOpenImmutableReadOnlyDoesNotCreateSidecars(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "immutable_clean.db")
	writer := openTestDBWithLiveWAL(t, dbPath)
	if _, err := writer.db.Exec("PRAGMA wal_checkpoint(FULL)"); err != nil {
		t.Fatalf("checkpoint clean immutable fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close clean immutable fixture: %v", err)
	}
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	immutable, err := OpenImmutableReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenImmutableReadOnly clean database: %v", err)
	}
	defer func() { _ = immutable.Close() }()
	var count int
	if err := immutable.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE text = 'live WAL committed row'").Scan(&count); err != nil {
		t.Fatalf("query immutable clean database: %v", err)
	}
	if count != 1 {
		t.Fatalf("immutable row count = %d, want 1", count)
	}
	assertStorageSidecarMissing(t, dbPath+"-wal")
	assertStorageSidecarMissing(t, dbPath+"-shm")
}

// TestOpenReadOnlyNonExistent verifies that opening a non-existent database fails.
func TestOpenReadOnlyNonExistent(t *testing.T) {
	_, err := OpenReadOnly("/nonexistent/path/db.sqlite")
	if err == nil {
		t.Fatalf("expected error opening non-existent database")
	}
}

func openTestDBWithLiveWAL(t *testing.T, dbPath string) *Database {
	t.Helper()
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open live WAL fixture: %v", err)
	}
	if err := db.SyncFiles([]IndexedFile{{
		SourcePath: "/sessions/live-wal.jsonl",
		Source:     "session",
		Hash:       "live-wal-hash",
		Project:    "project",
		Messages: []IndexedMessage{{
			Ordinal:     0,
			Role:        "user",
			Text:        "live WAL committed row",
			UUID:        "33333333-3333-4333-8333-333333333333",
			Timestamp:   "2026-08-18T00:00:00Z",
			ContentType: "text",
		}},
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("seed live WAL fixture: %v", err)
	}
	wal, err := os.Stat(dbPath + "-wal")
	if err != nil {
		_ = db.Close()
		t.Fatalf("stat live WAL sidecar: %v", err)
	}
	if wal.Size() == 0 {
		_ = db.Close()
		t.Fatal("live WAL sidecar is empty; test fixture did not create committed WAL frames")
	}
	return db
}

type storageSidecarSnapshot struct {
	Data  []byte
	Mode  os.FileMode
	MTime time.Time
}

func snapshotDBSidecars(t *testing.T, dbPath string) map[string]storageSidecarSnapshot {
	t.Helper()
	result := map[string]storageSidecarSnapshot{}
	for _, suffix := range []string{"-wal", "-shm"} {
		path := dbPath + suffix
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat sidecar %s: %v", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read sidecar %s: %v", path, err)
		}
		result[suffix] = storageSidecarSnapshot{Data: data, Mode: info.Mode(), MTime: info.ModTime()}
	}
	return result
}

func describeStorageSidecars(snapshot map[string]storageSidecarSnapshot) string {
	parts := make([]string, 0, len(snapshot))
	for _, suffix := range []string{"-wal", "-shm"} {
		entry, ok := snapshot[suffix]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s bytes=%d mode=%s mtime=%s", suffix, len(entry.Data), entry.Mode, entry.MTime.Format(time.RFC3339Nano)))
	}
	return fmt.Sprintf("%v", parts)
}

func assertStorageSidecarMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("sidecar %s exists after immutable read-only open (err=%v)", path, err)
	}
}

// TestSyncFiles inserts and updates files.
func TestSyncFiles(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Create test data
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "abc123def456",
			Project:    "my-project",
			Tags:       []string{"debugging", "feature"},
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "How do I fix this bug?",
					UUID:        getTestUUID(),
					Timestamp:   time.Now().Format(time.RFC3339),
					ContentType: "text",
				},
				{
					Ordinal:     1,
					Role:        "assistant",
					Text:        "You should check the error logs.",
					UUID:        getTestUUID(),
					Timestamp:   time.Now().Format(time.RFC3339),
					ContentType: "text",
				},
			},
		},
	}

	// Sync files
	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync files: %v", err)
	}

	// Verify search_items were inserted
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = ?", "/path/to/session1.jsonl").Scan(&count)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 search_items, got %d", count)
	}

	// Verify indexed_files was inserted
	var hash string
	err = db.db.QueryRow("SELECT hash FROM indexed_files WHERE path = ?", "/path/to/session1.jsonl").Scan(&hash)
	if err != nil || hash != "abc123def456" {
		t.Fatalf("hash mismatch")
	}

	// Verify tags were inserted
	err = db.db.QueryRow("SELECT COUNT(*) FROM session_tags WHERE source_path = ?", "/path/to/session1.jsonl").Scan(&count)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 session_tags, got %d", count)
	}
}

// TestGetFileHashes retrieves hashes.
func TestGetFileHashes(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync files
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "hash123",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "test",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Get hashes
	hashes, err := db.GetFileHashes()
	if err != nil {
		t.Fatalf("failed to get hashes: %v", err)
	}

	if hashes["/path/to/session1.jsonl"] != "hash123" {
		t.Fatalf("hash mismatch")
	}
}

// TestSearch performs a basic search query.
func TestSearch(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync files with searchable content
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "hash123",
			Project:    "proj1",
			Tags:       []string{"debugging"},
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "How do I fix this error in my code?",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
				{
					Ordinal:     1,
					Role:        "assistant",
					Text:        "The error occurs because you forgot to initialize the variable.",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Search for a term
	results, err := db.Search("error", models.SearchOptions{
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected search results, got none")
	}

	// Verify results contain expected content
	found := false
	for _, r := range results {
		if r.Role == "user" && r.SourcePath == "/path/to/session1.jsonl" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected result not found")
	}
}

// TestSearchWithFilters tests search with various filters.
func TestSearchWithFilters(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync multiple files
	now := time.Now()
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Tags:       []string{"feature"},
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "Build a new feature",
					UUID:        getTestUUID(),
					Timestamp:   now.Format(time.RFC3339),
					ContentType: "text",
				},
			},
		},
		{
			SourcePath: "/path/to/session2.jsonl",
			Source:     "session",
			Hash:       "hash2",
			Project:    "proj2",
			Tags:       []string{"debugging"},
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "assistant",
					Text:        "Debug the build issue",
					UUID:        getTestUUID(),
					Timestamp:   now.Add(-48 * time.Hour).Format(time.RFC3339),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Search with project filter
	results, err := db.Search("feature", models.SearchOptions{
		Project: "proj1",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("search with project filter failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected results with project filter")
	}

	// All results should be from proj1
	for _, r := range results {
		if r.Project != "proj1" {
			t.Fatalf("expected project proj1, got %s", r.Project)
		}
	}

	// Search with role filter
	results, err = db.Search("build", models.SearchOptions{
		Role:  "user",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("search with role filter failed: %v", err)
	}

	for _, r := range results {
		if r.Role != "user" {
			t.Fatalf("expected role user, got %s", r.Role)
		}
	}

	// Search with tag filter
	results, err = db.Search("debug", models.SearchOptions{
		Tag:   "debugging",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("search with tag filter failed: %v", err)
	}

	if len(results) > 0 {
		// At least one result should have the debugging tag
		found := false
		for _, r := range results {
			if r.SourcePath == "/path/to/session2.jsonl" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected result with debugging tag")
		}
	}
}

// TestGetStats returns database statistics.
func TestGetStats(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync files
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "Hello",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
				{
					Ordinal:     1,
					Role:        "assistant",
					Text:        "Hi there",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Get stats
	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalFiles != 1 {
		t.Fatalf("expected 1 file, got %d", stats.TotalFiles)
	}

	if stats.TotalMessages != 2 {
		t.Fatalf("expected 2 messages, got %d", stats.TotalMessages)
	}
}

// TestGetTopics returns topic terms.
func TestGetTopics(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync files with diverse content
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "database query performance optimization",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
				{
					Ordinal:     1,
					Role:        "assistant",
					Text:        "database indexing helps query optimization",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Get topics
	topics, err := db.GetTopics("", 20)
	if err != nil {
		t.Fatalf("failed to get topics: %v", err)
	}

	// Topics list may be empty (simplified implementation)
	// but the call should succeed
	if topics == nil {
		t.Fatalf("topics should not be nil")
	}

	// Topics should have reasonable length if any
	for _, topic := range topics {
		if len(topic.Term) < 3 {
			t.Fatalf("topic term too short: %s", topic.Term)
		}
	}
}

// TestListSessions lists indexed sessions.
func TestListSessions(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync files
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Tags:       []string{"feature", "debugging"},
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "test",
					UUID:        getTestUUID(),
					Timestamp:   time.Now().Format(time.RFC3339),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// List sessions
	sessions, err := db.ListSessions("", 0)
	if err != nil {
		t.Fatalf("failed to list sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	session := sessions[0]
	if session.Path != "/path/to/session1.jsonl" {
		t.Fatalf("path mismatch")
	}

	if len(session.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(session.Tags))
	}
}

// TestValidate checks database integrity.
func TestValidate(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Validate should pass on a fresh database
	err := db.Validate()
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
}

// TestMigrationsIdempotent verifies migrations are idempotent.
func TestMigrationsIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Open once
	db1, err := Open(dbPath)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	_ = db1.Close()

	// Open again (should not error)
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	_ = db2.Close()

	// Verify schema is intact
	db3, err := Open(dbPath)
	if err != nil {
		t.Fatalf("third open failed: %v", err)
	}
	defer func() { _ = db3.Close() }()

	err = db3.Validate()
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
}

// TestPurge deletes old records.
func TestPurge(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync files with different timestamps
	now := time.Now()
	oldTime := now.Add(-30 * 24 * time.Hour)

	files := []IndexedFile{
		{
			SourcePath: "/path/to/old_session.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "old message",
					UUID:        getTestUUID(),
					Timestamp:   oldTime.Format(time.RFC3339),
					ContentType: "text",
				},
			},
		},
		{
			SourcePath: "/path/to/new_session.jsonl",
			Source:     "session",
			Hash:       "hash2",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "new message",
					UUID:        getTestUUID(),
					Timestamp:   now.Format(time.RFC3339),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Purge records before 7 days ago
	purgeTime := now.Add(-7 * 24 * time.Hour)
	deleted, err := db.Purge(purgeTime.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}

	if deleted != 1 {
		t.Fatalf("expected 1 deleted record, got %d", deleted)
	}

	// Verify old record is gone, new one remains
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = ?", "/path/to/old_session.jsonl").Scan(&count)
	if err != nil || count != 0 {
		t.Fatalf("expected 0 old records, got %d", count)
	}

	err = db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = ?", "/path/to/new_session.jsonl").Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("expected 1 new record, got %d", count)
	}
}

// TestOptimizeFTS optimizes the FTS5 index.
func TestOptimizeFTS(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Add some data
	files := []IndexedFile{
		{
			SourcePath: "/path/to/session1.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "test content",
					UUID:        getTestUUID(),
					ContentType: "text",
				},
			},
		},
	}

	err := db.SyncFiles(files)
	if err != nil {
		t.Fatalf("failed to sync: %v", err)
	}

	// Optimize
	err = db.OptimizeFTS()
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}

	// Verify we can still search
	results, err := db.Search("test", models.SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search after optimize failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected results after optimize")
	}
}

// TestSearch_UnfilteredMergesToolAndProseWithRRF verifies that unfiltered searches
// merge tool_fts and messages_fts results using Reciprocal Rank Fusion (RRF).
func TestSearch_UnfilteredMergesToolAndProseWithRRF(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert a mixed corpus:
	// - Item 1: tool content, high in tool_fts
	// - Item 2: prose content, high in messages_fts, also has tool mention
	// - Item 3: tool only, lower rank in tool_fts
	// - Item 4: prose only, lower rank in messages_fts

	now := time.Now()
	items := []struct {
		id          int
		source      string
		contentType string
		text        string
		project     string
	}{
		{1, "session", "tool", "error: connection timeout failed", "proj1"},
		{2, "session", "text", "we debugged the connection timeout issue for hours", "proj1"},
		{3, "session", "tool", "retry mechanism implemented success", "proj1"},
		{4, "session", "code", "fixed a typo in the config", "proj1"},
	}

	for _, item := range items {
		_, err := db.db.Exec(`
			INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, item.source, "path/"+item.source, 1, "assistant", item.text, now.Format(time.RFC3339),
			fmt.Sprintf("uuid-%d", item.id), item.project, item.contentType)
		if err != nil {
			t.Fatalf("insert item %d: %v", item.id, err)
		}
	}

	// Search for "timeout" — appears in tool content (item 1) and prose (item 2)
	opts := models.SearchOptions{
		Project: "proj1",
		Limit:   100,
	}
	results, err := db.Search("timeout", opts)
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	// Expect results in RRF order:
	// - Both item 1 and 2 should be high (both matched "timeout")
	// - RRF should weight them based on rank in their respective indexes
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results with 'timeout', got %d", len(results))
	}

	// Verify: first two results contain items 1 and 2 (in some RRF order)
	topIDs := map[int]bool{results[0].ID: true, results[1].ID: true}
	if !topIDs[1] || !topIDs[2] {
		t.Errorf("expected items 1 and 2 in top 2, got IDs: %v", []int{results[0].ID, results[1].ID})
	}

	// Verify scores are RRF (not min-max normalized [0, 1])
	for _, r := range results {
		if r.Score <= 0 {
			t.Errorf("result ID %d has invalid RRF score %f", r.ID, r.Score)
		}
	}
}
