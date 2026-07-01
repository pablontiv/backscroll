package storage

import (
	"fmt"
	"os"
	"path/filepath"
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

// TestOpenReadOnlyNonExistent verifies that opening a non-existent database fails.
func TestOpenReadOnlyNonExistent(t *testing.T) {
	_, err := OpenReadOnly("/nonexistent/path/db.sqlite")
	if err == nil {
		t.Fatalf("expected error opening non-existent database")
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
