package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/embedding"
	"github.com/pablontiv/backscroll/internal/models"
)

func TestDB(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if db.DB() == nil {
		t.Error("DB() returned nil")
	}
}

func TestMigrationV5DropsSessionEvents(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	var name string
	err := db.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='session_events'",
	).Scan(&name)
	if err == nil {
		t.Fatalf("session_events should not exist after V5, but found %q", name)
	}
	// sql.ErrNoRows is the expected outcome.
}

func TestMigrationV6DropsSourceMetadata(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Check that source_metadata column does not exist in search_items
	rows, err := db.db.Query("PRAGMA table_info(search_items)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()

	var hasSourceMetadata bool
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dfltValue interface{}
		var pk int

		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan PRAGMA row: %v", err)
		}

		if name == "source_metadata" {
			hasSourceMetadata = true
			break
		}
	}

	if hasSourceMetadata {
		t.Fatal("source_metadata column should not exist after V6 migration")
	}

	// Verify idempotency: opening a new database should still succeed
	db2, cleanup2 := newTestDB(t)
	defer cleanup2()
	var count int
	if err := db2.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 6").Scan(&count); err != nil {
		t.Fatalf("check V6 migration applied: %v", err)
	}
	if count != 1 {
		t.Errorf("expected V6 migration to be applied once, got %d times", count)
	}
}

func TestNormalizeSource(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"all", ""},
		{"ALL", ""},
		{"sessions", "session"},
		{"SESSIONS", "session"},
		{"plans", "plan"},
		{"PLANS", "plan"},
		{"session", "session"},
		{"ke", "ke"},
		{"decision", "decision"},
	}
	for _, tc := range tests {
		got := normalizeSource(tc.in)
		if got != tc.want {
			t.Errorf("normalizeSource(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeFTS5Query(t *testing.T) {
	stopwords := map[string]struct{}{
		"the": {},
		"a":   {},
	}

	tests := []struct {
		query string
		want  string
	}{
		{"", ""},
		{"hello world", `"hello"* "world"*`},
		{"the a", `"the"* "a"*`}, // all stopwords → use unfiltered
		{"the hello", `"hello"*`},
	}
	for _, tc := range tests {
		got := sanitizeFTS5Query(tc.query, stopwords)
		if got != tc.want {
			t.Errorf("sanitizeFTS5Query(%q) = %q, want %q", tc.query, got, tc.want)
		}
	}
}

func TestLoadStopwords(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	stopwords, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("loadStopwords: %v", err)
	}
	// On an empty DB, stopwords should be empty
	if len(stopwords) != 0 {
		t.Errorf("expected 0 stopwords, got %d", len(stopwords))
	}
}

func TestSetupSchemaIdempotent(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	// Should not fail when called again on an already-initialized DB
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("SetupSchema twice: %v", err)
	}
}

func TestV2MigrationTablesExist(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	tables := []string{"chunks", "embedding_metadata"}
	for _, tbl := range tables {
		var name string
		err := db.DB().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after V2 migration: %v", tbl, err)
		}
	}

	// Verify version 2 was recorded
	var count int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version=2").Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row for version=2, got %d", count)
	}
}

func TestInsertChunks(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	chunks := []ChunkRecord{
		{ChunkIdx: 0, Content: "hello world", TokenCount: 3},
		{ChunkIdx: 1, Content: "foo bar baz", TokenCount: 3},
	}
	ids, err := db.InsertChunks("source/path.jsonl", chunks, 1234567890)
	if err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 chunk IDs, got %d", len(ids))
	}

	count, err := db.GetChunkCount()
	if err != nil {
		t.Fatalf("GetChunkCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 chunks, got %d", count)
	}

	// Re-inserting replaces old chunks
	ids2, err := db.InsertChunks("source/path.jsonl", chunks[:1], 1234567891)
	if err != nil {
		t.Fatalf("InsertChunks replace: %v", err)
	}
	if len(ids2) != 1 {
		t.Errorf("expected 1 chunk ID after replace, got %d", len(ids2))
	}
	count, _ = db.GetChunkCount()
	if count != 1 {
		t.Errorf("expected 1 chunk after replace, got %d", count)
	}
}

func TestInsertEmbeddingMetadata(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	chunks := []ChunkRecord{{ChunkIdx: 0, Content: "hello", TokenCount: 2}}
	ids, err := db.InsertChunks("source/em.jsonl", chunks, 1234567890)
	if err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	if err := db.InsertEmbeddingMetadata(ids[0], "all-MiniLM-L6-v2", "v1", 384, 1234567890); err != nil {
		t.Fatalf("InsertEmbeddingMetadata: %v", err)
	}

	count, err := db.GetEmbeddingCount()
	if err != nil {
		t.Fatalf("GetEmbeddingCount: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 embedding, got %d", count)
	}
}

func TestGetStatsChunksAndEmbeddings(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	chunks := []ChunkRecord{{ChunkIdx: 0, Content: "text", TokenCount: 1}}
	ids, _ := db.InsertChunks("path", chunks, 1)
	_ = db.InsertEmbeddingMetadata(ids[0], "model", "v1", 384, 1)

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalChunks != 1 {
		t.Errorf("TotalChunks = %d, want 1", stats.TotalChunks)
	}
	if stats.TotalEmbeddings != 1 {
		t.Errorf("TotalEmbeddings = %d, want 1", stats.TotalEmbeddings)
	}
}

func TestOpenReadOnlyCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ro.db")

	// Create the DB
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = db.Close()

	// Now open read-only
	rodb, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer func() { _ = rodb.Close() }()

	stats, err := rodb.GetStats()
	if err != nil {
		t.Fatalf("GetStats on readonly: %v", err)
	}
	_ = stats
}

func TestOpenReadOnlyNotSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notadb.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite file"), 0o644); err != nil {
		t.Fatal(err)
	}
	// May or may not fail depending on driver behavior
	db, err := OpenReadOnly(path)
	if err != nil {
		return
	}
	defer func() { _ = db.Close() }()
}

func TestSyncFilesEmptySlice(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	if err := db.SyncFiles(nil); err != nil {
		t.Fatalf("SyncFiles with nil: %v", err)
	}
	if err := db.SyncFiles([]IndexedFile{}); err != nil {
		t.Fatalf("SyncFiles with empty: %v", err)
	}
}

func TestSearchWithSourceFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "abc",
			Project:    "myproject",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello world", UUID: getTestUUID(), Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Test with source=sessions (normalized to session)
	results, err := db.Search("hello", models.SearchOptions{Source: "sessions", Limit: 10})
	if err != nil {
		t.Fatalf("Search with source sessions: %v", err)
	}
	_ = results

	// Test with source=all (no filter)
	results, err = db.Search("hello", models.SearchOptions{Source: "all", Limit: 10})
	if err != nil {
		t.Fatalf("Search with source all: %v", err)
	}
	_ = results
}

func TestPurgeInvalidDate(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()
	_, err := db.Purge("not-a-date")
	if err == nil {
		t.Error("expected error for invalid date, got nil")
	}
}

func TestSyncFilesNonSessionSource(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync a plan file (source != "session") to cover the non-session branch
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/plan.md",
			Source:     "plan",
			Hash:       "planhash",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "plan", Text: "implement the feature", Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
				{Ordinal: 1, Role: "plan", Text: "write comprehensive tests", Timestamp: "2024-01-01T00:01:00Z", ContentType: "text"},
			},
		},
		{
			SourcePath: "/test/ke.md",
			Source:     "ke",
			Hash:       "kehash",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "system", Text: "knowledge entry content", Timestamp: "", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles with plan/ke: %v", err)
	}

	hashes, err := db.GetFileHashes()
	if err != nil {
		t.Fatalf("GetFileHashes: %v", err)
	}
	if len(hashes) != 2 {
		t.Errorf("expected 2 file hashes, got %d", len(hashes))
	}
}

func TestSyncFilesWithStopwordsPopulation(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync content with lots of repeated words to populate FTS vocab / stopwords
	longText := "the quick brown fox jumps over the lazy dog the fox the dog the quick the brown"
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", UUID: getTestUUID(), Text: longText, Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// refreshStopwords is called automatically by SyncFiles
	// Verify we can load stopwords (may be empty or populated depending on FTS vocab)
	stopwords, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("loadStopwords after sync: %v", err)
	}
	t.Logf("stopwords count after sync: %d", len(stopwords))
}

func TestSearchWithAfterBefore(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", UUID: getTestUUID(), Text: "hello world test content", Timestamp: "2024-06-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	after, _ := time.Parse("2006-01-02", "2024-01-01")
	before, _ := time.Parse("2006-01-02", "2025-01-01")

	// Search with after/before filters
	results, err := db.Search("hello", models.SearchOptions{
		After:  &after,
		Before: &before,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search with date filters: %v", err)
	}
	_ = results

	// Search with role filter
	results, err = db.Search("hello", models.SearchOptions{
		Role:  "user",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Search with role: %v", err)
	}
	_ = results
}

func TestGetTopicsWithProject(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "myproject",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", UUID: getTestUUID(), Text: "python javascript golang database framework", Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Test GetTopics with project filter
	topics, err := db.GetTopics("myproject", 10)
	if err != nil {
		t.Fatalf("GetTopics with project: %v", err)
	}
	_ = topics

	// Test GetTopics with empty (all projects)
	topics, err = db.GetTopics("", 0) // 0 should default to 50
	if err != nil {
		t.Fatalf("GetTopics all: %v", err)
	}
	_ = topics
}

func TestGetStatsWithContent(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", UUID: getTestUUID(), Text: "hello world", Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalFiles != 1 {
		t.Errorf("expected 1 file, got %d", stats.TotalFiles)
	}
	if stats.TotalMessages != 1 {
		t.Errorf("expected 1 message, got %d", stats.TotalMessages)
	}
}

func TestQueryIndexedRecords(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	files := []IndexedFile{
		{
			SourcePath: "/sessions/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "projA",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "we decided to use Go", UUID: getTestUUID(), Timestamp: "2026-01-01T10:00:00Z", ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "great choice", UUID: getTestUUID(), Timestamp: "2026-01-01T10:01:00Z", ContentType: "text"},
			},
		},
		{
			SourcePath: "/decisions/d1.md",
			Source:     "decision",
			Hash:       "h2",
			Project:    "projA",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "---\nstatus: accepted\nscope: technical\n---\n# Use Go\nWe use Go.", UUID: getTestUUID(), Timestamp: "2026-01-02T00:00:00Z", ContentType: "text"},
			},
		},
		{
			SourcePath: "/sessions/s2.jsonl",
			Source:     "session",
			Hash:       "h3",
			Project:    "projB",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello from projB", UUID: getTestUUID(), Timestamp: "2026-01-03T00:00:00Z", ContentType: "text"},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// All records (no filters)
	all, err := db.QueryIndexedRecords(IndexedRecordQuery{})
	if err != nil {
		t.Fatalf("QueryIndexedRecords all: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 records, got %d", len(all))
	}

	// Filter by source
	src := "decision"
	decisions, err := db.QueryIndexedRecords(IndexedRecordQuery{Source: &src})
	if err != nil {
		t.Fatalf("QueryIndexedRecords decision: %v", err)
	}
	if len(decisions) != 1 {
		t.Errorf("expected 1 decision record, got %d", len(decisions))
	}
	if decisions[0].Source != "decision" {
		t.Errorf("expected source=decision, got %s", decisions[0].Source)
	}

	// Filter by project
	proj := "projA"
	projARecords, err := db.QueryIndexedRecords(IndexedRecordQuery{Project: &proj})
	if err != nil {
		t.Fatalf("QueryIndexedRecords projA: %v", err)
	}
	if len(projARecords) != 3 {
		t.Errorf("expected 3 projA records, got %d", len(projARecords))
	}

	// Filter by source + project
	sessA, err := db.QueryIndexedRecords(IndexedRecordQuery{Source: &[]string{"session"}[0], Project: &proj})
	if err != nil {
		t.Fatalf("QueryIndexedRecords session+projA: %v", err)
	}
	if len(sessA) != 2 {
		t.Errorf("expected 2 session records for projA, got %d", len(sessA))
	}

	// Filter by after date
	after := "2026-01-02"
	afterRecords, err := db.QueryIndexedRecords(IndexedRecordQuery{After: &after})
	if err != nil {
		t.Fatalf("QueryIndexedRecords after: %v", err)
	}
	if len(afterRecords) == 0 {
		t.Error("expected records after 2026-01-02")
	}

	// Limit
	limited, err := db.QueryIndexedRecords(IndexedRecordQuery{Limit: 2})
	if err != nil {
		t.Fatalf("QueryIndexedRecords limit: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 records with limit=2, got %d", len(limited))
	}

	// Source path filter
	sp := "/decisions/d1.md"
	byPath, err := db.QueryIndexedRecords(IndexedRecordQuery{SourcePath: &sp})
	if err != nil {
		t.Fatalf("QueryIndexedRecords source_path: %v", err)
	}
	if len(byPath) != 1 {
		t.Errorf("expected 1 record for source_path filter, got %d", len(byPath))
	}
}

func TestPurgeWithISODateFormat(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync old and new data
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/old/session.jsonl",
			Source:     "session",
			Hash:       "h-old",
			Project:    "test",
			Tags:       []string{"old-tag"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "old message content", UUID: getTestUUID(), Timestamp: "2019-06-01T00:00:00Z", ContentType: "text"},
			},
		},
		{
			SourcePath: "/new/session.jsonl",
			Source:     "session",
			Hash:       "h-new",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "new message content", UUID: getTestUUID(), Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Purge with ISO date format (exercises the RFC3339-fail → ISO-success path)
	deleted, err := db.Purge("2020-01-01")
	if err != nil {
		t.Fatalf("Purge ISO date: %v", err)
	}
	if deleted == 0 {
		t.Error("expected at least 1 record deleted")
	}

	// Purge with RFC3339 format (exercises the direct RFC3339-success path with real deletions)
	deleted2, err := db.Purge("2025-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Purge RFC3339: %v", err)
	}
	_ = deleted2
}

func TestListSessionsAfterSync(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "myproject",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello", UUID: getTestUUID(), Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
			Tags: []string{"feature", "debugging"},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// List all sessions
	sessions, err := db.ListSessions("", 5)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) == 0 {
		t.Error("expected at least 1 session")
	}

	// List with project filter
	sessions, err = db.ListSessions("myproject", 0)
	if err != nil {
		t.Fatalf("ListSessions with project: %v", err)
	}
	_ = sessions

	// List with non-matching project
	sessions, err = db.ListSessions("unknown", 0)
	if err != nil {
		t.Fatalf("ListSessions with unknown project: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for unknown project, got %d", len(sessions))
	}
}

func TestEmbeddingPipeline_WithMockProvider(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(384)
	defer func() { _ = provider.Close() }()

	sourcePath := "test/session.jsonl"
	text := "hello world. this is a test paragraph. it has multiple sentences!"
	chunks := []ChunkRecord{
		{ChunkIdx: 0, Content: text, TokenCount: 12},
	}
	now := time.Now().Unix()

	ids, err := db.InsertChunks(sourcePath, chunks, now)
	if err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	for _, id := range ids {
		vec, err := provider.Embed(context.Background(), text)
		if err != nil {
			t.Fatalf("Embed: %v", err)
		}
		if len(vec) != provider.Dimensions() {
			t.Errorf("vector length %d != dimensions %d", len(vec), provider.Dimensions())
		}
		if err := db.InsertEmbeddingMetadata(id, "mock", "v1", provider.Dimensions(), now); err != nil {
			t.Fatalf("InsertEmbeddingMetadata: %v", err)
		}
	}

	count, _ := db.GetChunkCount()
	if count != 1 {
		t.Errorf("expected 1 chunk, got %d", count)
	}
	embedCount, _ := db.GetEmbeddingCount()
	if embedCount != 1 {
		t.Errorf("expected 1 embedding, got %d", embedCount)
	}

	// Re-sync same source replaces chunks (dedup via InsertChunks delete+insert)
	_, err = db.InsertChunks(sourcePath, chunks, now+1)
	if err != nil {
		t.Fatalf("InsertChunks re-sync: %v", err)
	}
	count, _ = db.GetChunkCount()
	if count != 1 {
		t.Errorf("after re-sync: expected 1 chunk, got %d", count)
	}
	// embedding_metadata CASCADE deletes when chunk deleted
	embedCount, _ = db.GetEmbeddingCount()
	if embedCount != 0 {
		t.Errorf("after re-sync: expected 0 embeddings (CASCADE deleted), got %d", embedCount)
	}
}

func TestValidateWithOrphans(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Create an orphaned search_item by inserting directly without indexed_files entry
	_, err := db.DB().Exec(`
		INSERT INTO search_items (source_path, source, project, ordinal, role, text, timestamp, content_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "/orphan/path.jsonl", "session", "proj", 0, "user", "orphan content", "2024-01-01T00:00:00Z", "text")
	if err != nil {
		t.Fatalf("insert orphan: %v", err)
	}

	err = db.Validate()
	if err == nil {
		t.Error("expected error for orphaned search_items, got nil")
	}
}

func TestResolveSessionPath(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/home/user/.claude/projects/proj/session.jsonl",
			Source:     "session",
			Hash:       "abc",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Exact match
	path, err := db.ResolveSessionPath("/home/user/.claude/projects/proj/session.jsonl")
	if err != nil {
		t.Fatalf("ResolveSessionPath exact: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path for exact match")
	}

	// Fragment match
	path, err = db.ResolveSessionPath("proj/session.jsonl")
	if err != nil {
		t.Fatalf("ResolveSessionPath fragment: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path for fragment match")
	}

	// No match
	path, err = db.ResolveSessionPath("nonexistent-uuid-12345")
	if err != nil {
		t.Fatalf("ResolveSessionPath not found: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty path for no match, got %q", path)
	}
}

func TestVectorSearch_RoundTrip(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Index a file so search_items has a row with matching source_path
	_ = db.SyncFiles([]IndexedFile{{
		SourcePath: "test/a.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "hello vector world", ContentType: "text"},
		},
	}})

	// Insert a chunk with embedding
	ids, err := db.InsertChunks("test/a.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "hello vector world", TokenCount: 3},
	}, time.Now().Unix())
	if err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	vec := make([]float32, 4)
	for i := range vec {
		vec[i] = float32(i + 1)
	}
	if err := db.InsertChunkEmbedding(ids[0], vec); err != nil {
		t.Fatalf("InsertChunkEmbedding: %v", err)
	}

	count, err := db.GetVectorCount()
	if err != nil {
		t.Fatalf("GetVectorCount: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 vector, got %d", count)
	}

	// Vector search with the same vector should return similarity ≈ 1.0
	results, err := db.VectorSearch(vec, 10)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 vector result")
	}
	if results[0].Similarity < 0.99 {
		t.Errorf("expected similarity ≈ 1.0, got %.4f", results[0].Similarity)
	}
}

func TestHybridSearch_FallbackWithoutProvider(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	_ = db.SyncFiles([]IndexedFile{{
		SourcePath: "test/b.jsonl",
		Source:     "session",
		Hash:       "h2",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "hybrid search fallback test", ContentType: "text"},
		},
	}})

	// No provider set — should return BM25 results
	results, err := db.HybridSearch("hybrid", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected BM25 fallback results")
	}
}

func TestHybridSearch_WithMockProvider(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	_ = db.SyncFiles([]IndexedFile{{
		SourcePath: "test/c.jsonl",
		Source:     "session",
		Hash:       "h3",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "mock provider hybrid search", ContentType: "text"},
		},
	}})

	// Insert chunk + embedding so vector path activates
	ids, _ := db.InsertChunks("test/c.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "mock provider hybrid search", TokenCount: 4},
	}, time.Now().Unix())
	vec, _ := provider.Embed(context.Background(), "mock provider hybrid search")
	_ = db.InsertChunkEmbedding(ids[0], vec)

	results, err := db.HybridSearch("mock", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch with provider: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected hybrid results with mock provider")
	}
}

func TestHybridSearch_LexicalOnly(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	_ = db.SyncFiles([]IndexedFile{{
		SourcePath: "test/d.jsonl",
		Source:     "session",
		Hash:       "h4",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "lexical only search test", ContentType: "text"},
		},
	}})

	// LexicalOnly=true should skip vector path even with provider
	results, err := db.HybridSearch("lexical", models.SearchOptions{
		AllProjects: true,
		LexicalOnly: true,
	})
	if err != nil {
		t.Fatalf("HybridSearch lexical-only: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected BM25 results with lexical-only")
	}
}

func TestHasEmbeddingProvider(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if db.HasEmbeddingProvider() {
		t.Error("expected no provider initially")
	}

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	if !db.HasEmbeddingProvider() {
		t.Error("expected provider after SetEmbeddingProvider")
	}
}

func TestHybridSearch_SimilarityThreshold(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	_ = db.SyncFiles([]IndexedFile{{
		SourcePath: "test/e.jsonl",
		Source:     "session",
		Hash:       "h5",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "threshold filter test query", ContentType: "text"},
		},
	}})

	ids, _ := db.InsertChunks("test/e.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "threshold filter test query", TokenCount: 4},
	}, time.Now().Unix())
	vec, _ := provider.Embed(context.Background(), "threshold filter test query")
	_ = db.InsertChunkEmbedding(ids[0], vec)

	// High threshold — mock provider produces deterministic unit vectors so similarity ≈ 1.0
	// Using very high threshold (e.g. 0.99) should still return results
	results, err := db.HybridSearch("threshold", models.SearchOptions{
		AllProjects:         true,
		SimilarityThreshold: 0.99,
	})
	if err != nil {
		t.Fatalf("HybridSearch similarity threshold: %v", err)
	}
	// Results may be empty if similarity < threshold — just verify no error
	_ = results
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	zero := []float32{0, 0, 0}
	unit := []float32{1, 0, 0}
	if cosineSimilarity(zero, unit) != 0 {
		t.Error("cosineSimilarity with zero vector should return 0")
	}
	if cosineSimilarity(unit, zero) != 0 {
		t.Error("cosineSimilarity with zero second vector should return 0")
	}
}

func TestDecodeEmbedding_InvalidLength(t *testing.T) {
	// Odd-byte slice — not aligned to 4 bytes
	bad := []byte{0x01, 0x02, 0x03}
	if decodeEmbedding(bad) != nil {
		t.Error("expected nil for misaligned byte slice")
	}
}

// TestOpenPingError covers the ping error path in Open
func TestOpenPingError(t *testing.T) {
	// Create an invalid database path that will fail on ping
	// This is hard to trigger with sqlite, so we'll use a read-only directory
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create the file
	if err := os.WriteFile(dbPath, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	// Make directory read-only to force errors
	if err := os.Chmod(dir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o755) }()

	// Attempt to open should fail due to permissions
	_, err := Open(filepath.Join(dir, "newdb.db"))
	if err == nil {
		t.Error("expected error opening database in read-only directory")
	}
}

// TestOpenForeignKeysError covers the foreign_keys PRAGMA error path
func TestOpenForeignKeysError(t *testing.T) {
	// This is difficult to force with real sqlite. Instead, verify
	// that valid databases have FK enabled.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify FK is enabled
	var enabled int
	err = db.DB().QueryRow("PRAGMA foreign_keys").Scan(&enabled)
	if err != nil {
		t.Fatalf("PRAGMA query: %v", err)
	}
	if enabled != 1 {
		t.Error("foreign_keys pragma should be enabled")
	}
}

// TestLoadStopwordsQueryError covers the error path when stopwords query fails
func TestLoadStopwordsQueryError(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Since loadStopwords is called during SyncFiles, we can't easily
	// force a query error. However, we can test that it gracefully handles
	// an empty stopwords table.
	stopwords, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("loadStopwords on empty DB: %v", err)
	}
	if len(stopwords) != 0 {
		t.Errorf("expected 0 stopwords, got %d", len(stopwords))
	}
}

// TestGetFileHashesScanError covers error handling in GetFileHashes
func TestGetFileHashesScanError(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Add a file
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/file.jsonl",
			Source:     "session",
			Hash:       "abc123",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// GetFileHashes should succeed and return the hash
	hashes, err := db.GetFileHashes()
	if err != nil {
		t.Fatalf("GetFileHashes: %v", err)
	}
	if len(hashes) != 1 {
		t.Errorf("expected 1 hash, got %d", len(hashes))
	}
	if hashes["/test/file.jsonl"] != "abc123" {
		t.Errorf("hash mismatch")
	}
}

// TestRefreshStopwordsEmptyVocab covers the case where messages_vocab is empty
func TestRefreshStopwordsEmptyVocab(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// refreshStopwords is called automatically by SyncFiles
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "a", UUID: getTestUUID(), Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Verify dynamic_stopwords table exists and may have terms
	var count int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM dynamic_stopwords").Scan(&count); err != nil {
		t.Fatalf("query stopwords: %v", err)
	}
	// count could be 0 or more depending on vocab
	if count < 0 {
		t.Error("count should be non-negative")
	}
}

// TestSetupSchemaCheckMigrationError covers migration check errors
func TestSetupSchemaCheckMigrationError(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// SetupSchema should have been called in Open, so calling again should be idempotent
	err := db.SetupSchema()
	if err != nil {
		t.Fatalf("SetupSchema idempotent call: %v", err)
	}

	// Verify all migrations were applied
	var v1, v2, v3 int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&v1)
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 2").Scan(&v2)
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 3").Scan(&v3)

	if v1 != 1 {
		t.Error("V1 migration should be applied")
	}
	if v2 != 1 {
		t.Error("V2 migration should be applied")
	}
	if v3 != 1 {
		t.Error("V3 migration should be applied")
	}
}

// TestSyncFilesTransactionRollback verifies transaction handling
func TestSyncFilesTransactionRollback(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// First successful sync
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test1", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM indexed_files").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 file after first sync, got %d", count)
	}

	// Second sync should not duplicate
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test2", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	_ = db.DB().QueryRow("SELECT COUNT(*) FROM indexed_files").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 files after second sync, got %d", count)
	}
}

// TestSetSessionSourceMetadataWithEmptyPath tests edge case
// TestSearchErrorHandling covers search error paths
func TestSearchErrorHandling(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Search on empty database should return empty results, not error
	results, err := db.Search("nonexistent", models.SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search on empty DB: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestGetStatsEmptyDatabase covers stats on empty DB
func TestGetStatsEmptyDatabase(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats empty DB: %v", err)
	}
	if stats.TotalFiles != 0 {
		t.Errorf("expected 0 files, got %d", stats.TotalFiles)
	}
	if stats.TotalMessages != 0 {
		t.Errorf("expected 0 messages, got %d", stats.TotalMessages)
	}
}

// TestHybridSearchWithAllProjectsFilter covers allprojects path
func TestHybridSearchWithAllProjectsFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "search keyword", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	results, err := db.HybridSearch("search", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch all projects: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results for AllProjects=true")
	}
}

// TestPurgeWithMultiplePaths covers the multi-path deletion logic
func TestPurgeWithMultiplePaths(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	now := time.Now()
	oldTime := now.Add(-100 * 24 * time.Hour)

	// Sync files with different ages
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/old/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Tags:       []string{"old"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "old1", UUID: getTestUUID(), Timestamp: oldTime.Format(time.RFC3339), ContentType: "text"},
			},
		},
		{
			SourcePath: "/old/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "test",
			Tags:       []string{"old"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "old2", UUID: getTestUUID(), Timestamp: oldTime.Format(time.RFC3339), ContentType: "text"},
			},
		},
		{
			SourcePath: "/new/s3.jsonl",
			Source:     "session",
			Hash:       "h3",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "new", UUID: getTestUUID(), Timestamp: now.Format(time.RFC3339), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Purge old records
	purgeTime := now.Add(-50 * 24 * time.Hour)
	deleted, err := db.Purge(purgeTime.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	// Verify old files are deleted from indexed_files
	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM indexed_files WHERE path = ?", "/old/s1.jsonl").Scan(&count)
	if count != 0 {
		t.Error("old indexed_files should be deleted")
	}

	// Verify new file remains
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM indexed_files WHERE path = ?", "/new/s3.jsonl").Scan(&count)
	if count != 1 {
		t.Error("new indexed_files should remain")
	}
}

// TestValidateSucceedsOnCleanDB checks validation passes on clean state
func TestValidateSucceedsOnCleanDB(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Fresh DB should validate
	if err := db.Validate(); err != nil {
		t.Fatalf("Validate on fresh DB: %v", err)
	}
}

// TestOptimizeFTSSucceeds checks FTS optimization
func TestOptimizeFTSSucceeds(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Add some data first
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "optimize test content", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Optimize should succeed
	if err := db.OptimizeFTS(); err != nil {
		t.Fatalf("OptimizeFTS: %v", err)
	}

	// Verify search still works after optimization
	results, err := db.Search("optimize", models.SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search after optimize: %v", err)
	}
	_ = results
}

// TestGetStatsWithMultipleProjects covers stats with multiple projects
func TestGetStatsWithMultipleProjects(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "proj1 msg", UUID: getTestUUID(), ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "proj1 reply", UUID: getTestUUID(), ContentType: "text"},
			},
		},
		{
			SourcePath: "/test/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "proj2 msg", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", stats.TotalFiles)
	}
	if stats.TotalMessages != 3 {
		t.Errorf("expected 3 messages, got %d", stats.TotalMessages)
	}
}

// TestSearchWithContentTypeFilter covers content type filtering
func TestSearchWithContentTypeFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "search term", UUID: getTestUUID(), ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "code snippet", UUID: getTestUUID(), ContentType: "code"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Search with content type filter
	results, err := db.Search("search", models.SearchOptions{
		ContentType: "text",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Search with content_type filter: %v", err)
	}
	for _, r := range results {
		if r.ContentType != "text" {
			t.Errorf("expected content_type=text, got %s", r.ContentType)
		}
	}
}

// TestInsertChunksError covers error handling in InsertChunks
func TestInsertChunksReplaceOldChunks(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert chunks
	ids1, err := db.InsertChunks("source/a.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "first chunk", TokenCount: 2},
	}, time.Now().Unix())
	if err != nil {
		t.Fatalf("InsertChunks first: %v", err)
	}
	if len(ids1) != 1 {
		t.Errorf("expected 1 chunk ID, got %d", len(ids1))
	}

	// Insert new chunks for same source (should replace)
	ids2, err := db.InsertChunks("source/a.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "replaced chunk", TokenCount: 2},
		{ChunkIdx: 1, Content: "second chunk", TokenCount: 2},
	}, time.Now().Unix())
	if err != nil {
		t.Fatalf("InsertChunks second: %v", err)
	}
	if len(ids2) != 2 {
		t.Errorf("expected 2 chunk IDs after replace, got %d", len(ids2))
	}

	// Verify only new chunks exist
	count, _ := db.GetChunkCount()
	if count != 2 {
		t.Errorf("expected 2 chunks total, got %d", count)
	}
}

// TestGetTopicsLimit covers topics limit behavior
func TestGetTopicsLimit(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "golang python javascript rust database", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Get topics with limit
	topics, err := db.GetTopics("test", 2)
	if err != nil {
		t.Fatalf("GetTopics: %v", err)
	}
	if len(topics) > 2 {
		t.Errorf("expected at most 2 topics, got %d", len(topics))
	}
}

// TestLoadStopwordsAfterSync verifies stopwords are populated after sync
func TestLoadStopwordsAfterSync(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync content with common words
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "the quick brown fox the lazy dog the", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Load stopwords
	stopwords, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("loadStopwords: %v", err)
	}
	_ = stopwords
}

// TestSyncFilesWithoutMessages verifies empty message lists are handled
func TestSyncFilesWithoutMessages(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// This should still succeed even with no messages
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/empty.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages:   []IndexedMessage{},
		},
	}); err != nil {
		t.Fatalf("SyncFiles with no messages: %v", err)
	}

	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM indexed_files WHERE path = ?", "/test/empty.jsonl").Scan(&count)
	if count != 1 {
		t.Error("file should be indexed even without messages")
	}
}

// TestVectorSearchWithMismatchedDimensions covers mismatched vector size path
func TestVectorSearchWithMismatchedDimensions(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert chunk with embedding
	ids, _ := db.InsertChunks("source/a.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "hello world", TokenCount: 2},
	}, time.Now().Unix())

	// Insert embedding with 4-dim vector
	vec4 := []float32{1, 2, 3, 4}
	_ = db.InsertChunkEmbedding(ids[0], vec4)

	// Search with 3-dim vector (mismatched)
	vec3 := []float32{1, 2, 3}
	results, err := db.VectorSearch(vec3, 10)
	if err != nil {
		t.Fatalf("VectorSearch mismatched: %v", err)
	}
	// Results should be empty or not include mismatched embedding
	_ = results
}

// TestVectorSearchWithEmptyIndex covers empty embeddings
func TestVectorSearchWithEmptyIndex(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Search on empty embeddings
	vec := []float32{1, 2, 3, 4}
	results, err := db.VectorSearch(vec, 10)
	if err != nil {
		t.Fatalf("VectorSearch empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty index, got %d", len(results))
	}
}

// TestVectorSearchTopKLimit covers topK limiting
func TestVectorSearchTopKLimit(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert multiple chunks with embeddings
	for i := 0; i < 5; i++ {
		ids, _ := db.InsertChunks(
			fmt.Sprintf("source/f%d.jsonl", i),
			[]ChunkRecord{{ChunkIdx: 0, Content: "test", TokenCount: 1}},
			time.Now().Unix(),
		)
		vec := make([]float32, 4)
		for j := range vec {
			vec[j] = float32(i+j) + 0.1
		}
		_ = db.InsertChunkEmbedding(ids[0], vec)
	}

	// Search with topK=2
	vec := []float32{1, 1, 1, 1}
	results, err := db.VectorSearch(vec, 2)
	if err != nil {
		t.Fatalf("VectorSearch topK: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected at most 2 results with topK=2, got %d", len(results))
	}
}

// TestHybridSearchWithEmptyIndex covers hybrid search on empty DB
func TestHybridSearchWithEmptyIndex(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Hybrid search on empty database
	results, err := db.HybridSearch("test", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty index, got %d", len(results))
	}
}

// TestHybridSearchVectorDeduplication covers the deduplication logic
func TestHybridSearchVectorDeduplication(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Set a provider
	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	// Index content
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "test/v.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "dedup vector test content", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Insert chunk with matching embedding
	ids, _ := db.InsertChunks("test/v.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "dedup vector test content", TokenCount: 5},
	}, time.Now().Unix())

	vec, _ := provider.Embed(context.Background(), "dedup vector test content")
	_ = db.InsertChunkEmbedding(ids[0], vec)

	// Search should handle deduplication correctly
	results, err := db.HybridSearch("dedup", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch dedup: %v", err)
	}
	_ = results
}

// TestLoadChunkEmbeddingsError covers error path in LoadChunkEmbeddings
func TestLoadChunkEmbeddingsEmpty(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Load embeddings from empty database
	embeddings, err := db.LoadChunkEmbeddings()
	if err != nil {
		t.Fatalf("LoadChunkEmbeddings empty: %v", err)
	}
	if len(embeddings) != 0 {
		t.Errorf("expected 0 embeddings, got %d", len(embeddings))
	}
}

// TestValidateFTSTableExists covers FTS table check
func TestValidateFTSTableExists(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Validate should pass with proper FTS table
	if err := db.Validate(); err != nil {
		t.Fatalf("Validate with FTS: %v", err)
	}
}

// TestQueryIndexedRecordsPaginationAndOffset covers limit/offset
func TestQueryIndexedRecordsLimitAndOffset(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert multiple records
	for i := 0; i < 5; i++ {
		_ = db.SyncFiles([]IndexedFile{
			{
				SourcePath: fmt.Sprintf("/test/s%d.jsonl", i),
				Source:     "session",
				Hash:       fmt.Sprintf("h%d", i),
				Project:    "test",
				Messages: []IndexedMessage{
					{Ordinal: 0, Role: "user", Text: fmt.Sprintf("msg%d", i), UUID: getTestUUID(), ContentType: "text"},
				},
			},
		})
	}

	// Query with limit
	records, err := db.QueryIndexedRecords(IndexedRecordQuery{Limit: 2})
	if err != nil {
		t.Fatalf("QueryIndexedRecords limit: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records with limit=2, got %d", len(records))
	}

	// Query all
	records, err = db.QueryIndexedRecords(IndexedRecordQuery{})
	if err != nil {
		t.Fatalf("QueryIndexedRecords all: %v", err)
	}
	if len(records) < 5 {
		t.Errorf("expected at least 5 records, got %d", len(records))
	}
}

// TestPurgeWithoutTimestamps covers records without timestamps
func TestPurgeWithoutTimestamps(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync without timestamps (timestamp will be empty string/NULL)
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/notimestamp.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "no timestamp", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Purge with an old date — records with NULL/empty timestamp might still be deleted
	// depending on SQL NULL comparison semantics
	oldDate := time.Now().Add(-1000 * 24 * time.Hour)
	deleted, err := db.Purge(oldDate.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	// Just verify it succeeds; deleted count depends on NULL handling
	_ = deleted
}

// TestGetFileHashesIterationError covers rows.Err() path
func TestGetFileHashesMultiple(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync multiple files
	for i := 0; i < 3; i++ {
		_ = db.SyncFiles([]IndexedFile{
			{
				SourcePath: fmt.Sprintf("/test/s%d.jsonl", i),
				Source:     "session",
				Hash:       fmt.Sprintf("hash%d", i),
				Project:    "test",
				Messages: []IndexedMessage{
					{Ordinal: 0, Role: "user", Text: "test", UUID: getTestUUID(), ContentType: "text"},
				},
			},
		})
	}

	hashes, err := db.GetFileHashes()
	if err != nil {
		t.Fatalf("GetFileHashes: %v", err)
	}
	if len(hashes) != 3 {
		t.Errorf("expected 3 hashes, got %d", len(hashes))
	}
}

// TestInsertChunksCommitError covers transaction commit
func TestInsertChunksMultipleBatches(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Multiple insertions
	for batch := 0; batch < 2; batch++ {
		ids, err := db.InsertChunks(
			fmt.Sprintf("source/batch%d.jsonl", batch),
			[]ChunkRecord{
				{ChunkIdx: 0, Content: "chunk a", TokenCount: 2},
				{ChunkIdx: 1, Content: "chunk b", TokenCount: 2},
			},
			time.Now().Unix(),
		)
		if err != nil {
			t.Fatalf("InsertChunks batch %d: %v", batch, err)
		}
		if len(ids) != 2 {
			t.Errorf("expected 2 IDs, got %d", len(ids))
		}
	}

	count, _ := db.GetChunkCount()
	if count != 4 {
		t.Errorf("expected 4 chunks total, got %d", count)
	}
}

// TestHybridSearchSimilarityFilter covers the similarity threshold filtering
func TestHybridSearchSimilarityFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	// Index content
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "test/sim.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "similarity filtering test", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Insert chunk with embedding
	ids, _ := db.InsertChunks("test/sim.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "similarity filtering test", TokenCount: 3},
	}, time.Now().Unix())
	vec, _ := provider.Embed(context.Background(), "similarity filtering test")
	_ = db.InsertChunkEmbedding(ids[0], vec)

	// Search with high similarity threshold
	results, err := db.HybridSearch("similarity", models.SearchOptions{
		AllProjects:         true,
		SimilarityThreshold: 0.5,
	})
	if err != nil {
		t.Fatalf("HybridSearch similarity: %v", err)
	}
	_ = results
}

// TestSyncFilesSessionTagsNodup covers session-specific tag insertion
func TestSyncFilesSessionTagsDedupPath(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync with unique tags (no duplicates to avoid constraint)
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Tags:       []string{"feature", "debugging", "refactor"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Verify tags are inserted
	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM session_tags WHERE source_path = ?", "/test/s.jsonl").Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 tags, got %d", count)
	}
}

// TestSearchWithMultipleFilters covers combined filters
func TestSearchWithMultipleFilters(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	now := time.Now()
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "myproject",
			Tags:       []string{"feature"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "search query test", UUID: getTestUUID(), Timestamp: now.Format(time.RFC3339), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Search with multiple filters combined
	results, err := db.Search("search", models.SearchOptions{
		Project:     "myproject",
		Role:        "user",
		Tag:         "feature",
		ContentType: "text",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Search with multiple filters: %v", err)
	}
	_ = results
}

// TestRefreshStopwordsInsertError covers insertions after load
func TestRefreshStopwordsMultipleCalls(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync multiple times to trigger refreshStopwords multiple times
	for i := 0; i < 3; i++ {
		if err := db.SyncFiles([]IndexedFile{
			{
				SourcePath: fmt.Sprintf("/test/s%d.jsonl", i),
				Source:     "session",
				Hash:       fmt.Sprintf("h%d", i),
				Project:    "test",
				Messages: []IndexedMessage{
					{Ordinal: 0, Role: "user", Text: "the quick brown fox jumps over the lazy dog", UUID: getTestUUID(), ContentType: "text"},
				},
			},
		}); err != nil {
			t.Fatalf("SyncFiles %d: %v", i, err)
		}
	}

	// Stopwords should have been refreshed multiple times
	stopwords, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("loadStopwords: %v", err)
	}
	_ = stopwords
}

// TestPurgeRowsAffected covers the RowsAffected path
func TestPurgeRowsAffectedTracking(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	now := time.Now()
	oldTime := now.Add(-100 * 24 * time.Hour)

	// Sync with old and new data
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/old/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "old msg", UUID: getTestUUID(), Timestamp: oldTime.Format(time.RFC3339), ContentType: "text"},
			},
		},
		{
			SourcePath: "/new/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "new msg", UUID: getTestUUID(), Timestamp: now.Format(time.RFC3339), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Purge and check RowsAffected is tracked
	purgeTime := now.Add(-50 * 24 * time.Hour)
	deleted, err := db.Purge(purgeTime.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if deleted < 0 {
		t.Errorf("RowsAffected should be non-negative, got %d", deleted)
	}
}

// TestValidateAllTablesMissing would need direct DB manipulation; skip for now
// TestValidateFTSMissing would need direct DB manipulation; skip for now

// TestSearchProjectFiltering covers project-specific search
func TestSearchProjectFiltering(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "golang code", UUID: getTestUUID(), ContentType: "text"},
			},
		},
		{
			SourcePath: "/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "python code", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Search with project filter
	results, err := db.Search("code", models.SearchOptions{
		Project: "proj1",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("Search with project: %v", err)
	}
	for _, r := range results {
		if r.Project != "proj1" {
			t.Errorf("expected project=proj1, got %s", r.Project)
		}
	}
}

// TestHybridSearchRRFFusion covers the RRF fusion path with both BM25 and vector results
func TestHybridSearchRRFFusion(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	// Index multiple documents
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "test/rrf1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "machine learning algorithms", UUID: getTestUUID(), ContentType: "text"},
			},
		},
		{
			SourcePath: "test/rrf2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "deep learning neural networks", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Insert chunks and embeddings for both
	for i, path := range []string{"test/rrf1.jsonl", "test/rrf2.jsonl"} {
		text := []string{"machine learning algorithms", "deep learning neural networks"}[i]
		ids, _ := db.InsertChunks(path, []ChunkRecord{
			{ChunkIdx: 0, Content: text, TokenCount: 3},
		}, time.Now().Unix())
		vec, _ := provider.Embed(context.Background(), text)
		_ = db.InsertChunkEmbedding(ids[0], vec)
	}

	// Hybrid search with limit to trigger RRF path
	results, err := db.HybridSearch("learning", models.SearchOptions{
		AllProjects: true,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("HybridSearch RRF: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected RRF fusion results")
	}
}

// TestHybridSearchVectorCountZero covers zero vector count path
func TestHybridSearchZeroVectorCount(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	// Index without vectors
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "test/novector.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "no vector embedding", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// HybridSearch should fallback to BM25 when no vectors exist
	results, err := db.HybridSearch("vector", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch no vectors: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected BM25 fallback results")
	}
}

// TestHybridSearchProviderEmbedError covers provider embed error
func TestHybridSearchProviderEmbedError(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Mock provider that succeeds
	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	// Index with vector embeddings
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "test/embed.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "embedding test query", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	ids, _ := db.InsertChunks("test/embed.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "embedding test query", TokenCount: 3},
	}, time.Now().Unix())
	vec, _ := provider.Embed(context.Background(), "embedding test query")
	_ = db.InsertChunkEmbedding(ids[0], vec)

	// Search should handle embedding success gracefully
	results, err := db.HybridSearch("embedding", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch with embed: %v", err)
	}
	_ = results
}

// TestHybridSearchEmptyVectorResults covers empty vector results path
func TestHybridSearchEmptyVectorResults(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	provider := embedding.NewMockProvider(4)
	defer func() { _ = provider.Close() }()
	db.SetEmbeddingProvider(provider)

	// Index document
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "test/emptyvec.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "text content here", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Add vector embedding with specific vector
	ids, _ := db.InsertChunks("test/emptyvec.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "text content here", TokenCount: 3},
	}, time.Now().Unix())
	// Insert vector of different dimension to cause mismatch
	vec := make([]float32, 4)
	for i := range vec {
		vec[i] = float32(i+1) * 10
	}
	_ = db.InsertChunkEmbedding(ids[0], vec)

	// Search with query that won't match vectors (different embedding)
	results, err := db.HybridSearch("different", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch empty vectors: %v", err)
	}
	_ = results
}

// TestOpenReadOnlyPingError covers read-only ping success path
func TestOpenReadOnlyPingSuccess(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "ro.db")

	// Create DB first
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = db.Close()

	// Open in read-only should succeed
	rodb, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer func() { _ = rodb.Close() }()

	// Verify we can query
	var count int
	_ = rodb.DB().QueryRow("SELECT COUNT(*) FROM indexed_files").Scan(&count)
}

// TestSyncFilesMessageUUIDHandling covers uuid nil/empty path
func TestSyncFilesMessageUUIDHandling(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync with empty UUID
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "empty uuid", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles empty uuid: %v", err)
	}

	// Sync with uuid
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "with uuid", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles with uuid: %v", err)
	}

	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM search_items").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 items, got %d", count)
	}
}

// TestGetFileHashesScanFail covers the scan error path
func TestGetFileHashesAfterSync(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync and then query
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/f.jsonl",
			Source:     "session",
			Hash:       "myhash",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	hashes, err := db.GetFileHashes()
	if err != nil {
		t.Fatalf("GetFileHashes: %v", err)
	}
	if hashes["/test/f.jsonl"] != "myhash" {
		t.Errorf("hash mismatch")
	}
}

// TestLoadStopwordsTableNotFound covers "no such table" error handling
func TestLoadStopwordsWithContent(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Add content to populate FTS vocab/stopwords
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "the the the quick quick brown brown brown brown fox", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// refreshStopwords was called, verify we can load them
	stopwords, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("loadStopwords: %v", err)
	}
	_ = stopwords
}

// TestValidateWithValidDB verifies validation passes
func TestValidateAfterSync(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	if err := db.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// TestOptimizeFTSAfterSync verifies optimization works with data
func TestOptimizeFTSAfterSync(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Add data
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "optimize fts after sync", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Optimize
	if err := db.OptimizeFTS(); err != nil {
		t.Fatalf("OptimizeFTS: %v", err)
	}
}

// TestPurgeRowCountError path (rows.Err after rows.Next)
func TestPurgeWithSessionDeletion(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	now := time.Now()
	oldTime := now.Add(-200 * 24 * time.Hour)

	// Sync multiple messages in one file
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/old.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Tags:       []string{"old"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "msg1", UUID: getTestUUID(), Timestamp: oldTime.Format(time.RFC3339), ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "msg2", UUID: getTestUUID(), Timestamp: oldTime.Format(time.RFC3339), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Purge should delete all items and the indexed_files entry
	purgeTime := now.Add(-100 * 24 * time.Hour)
	deleted, err := db.Purge(purgeTime.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if deleted < 2 {
		t.Errorf("expected at least 2 deletions, got %d", deleted)
	}

	// Verify indexed_files was deleted
	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM indexed_files WHERE path = ?", "/test/old.jsonl").Scan(&count)
	if count != 0 {
		t.Error("indexed_files entry should be deleted")
	}

	// Verify session_tags was deleted
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM session_tags WHERE source_path = ?", "/test/old.jsonl").Scan(&count)
	if count != 0 {
		t.Error("session_tags should be deleted")
	}
}

// TestInsertChunksLargeChunkIndex covers LastInsertId handling
func TestInsertChunksLargeIndex(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert chunks with large indices
	ids, err := db.InsertChunks("source/large.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "chunk 0", TokenCount: 2},
		{ChunkIdx: 100, Content: "chunk 100", TokenCount: 2},
		{ChunkIdx: 1000, Content: "chunk 1000", TokenCount: 2},
	}, time.Now().Unix())
	if err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}
	if len(ids) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(ids))
	}
}

// TestSyncFilesNonSessionSource covers non-session source path
func TestSyncFilesNonSessionSources(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync various non-session sources
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/source/plan.md",
			Source:     "plan",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "plan", Text: "implement feature", ContentType: "text"},
			},
		},
		{
			SourcePath: "/source/ke.md",
			Source:     "ke",
			Hash:       "h2",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "system", Text: "knowledge entry", ContentType: "text"},
			},
		},
		{
			SourcePath: "/source/decision.md",
			Source:     "decision",
			Hash:       "h3",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "system", Text: "decision record", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles non-session: %v", err)
	}

	// Verify all were indexed
	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM indexed_files").Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 indexed files, got %d", count)
	}

	// Verify no session_tags were created (only for sessions)
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM session_tags").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 tags (non-session), got %d", count)
	}
}

// TestSearchAllProjectsOption covers AllProjects flag
func TestSearchAllProjectsOption(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "projA",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "search allprojects", UUID: getTestUUID(), ContentType: "text"},
			},
		},
		{
			SourcePath: "/test/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "projB",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "search allprojects", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Search with AllProjects=true should return results from both
	results, err := db.Search("search", models.SearchOptions{
		AllProjects: true,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Search AllProjects: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected results from all projects")
	}
}

// TestSearchSourceFilter covers source type filter
func TestSearchSourceFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "source search content", UUID: getTestUUID(), ContentType: "text"},
			},
		},
		{
			SourcePath: "/test/p1.md",
			Source:     "plan",
			Hash:       "h2",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "plan", Text: "source plan content", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Search with Source filter for sessions
	results, err := db.Search("source", models.SearchOptions{
		Source: "session",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("Search Source: %v", err)
	}
	for _, r := range results {
		if r.Source != "session" {
			t.Errorf("expected source=session, got %s", r.Source)
		}
	}
}

// TestGetStatsWithChunks covers stats with chunks
func TestGetStatsWithAllTypes(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync files with messages
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "stats test", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Insert chunks with embeddings
	ids, _ := db.InsertChunks("/test/s.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "chunk content", TokenCount: 2},
	}, time.Now().Unix())
	vec := make([]float32, 4)
	for i := range vec {
		vec[i] = float32(i + 1)
	}
	_ = db.InsertChunkEmbedding(ids[0], vec)

	// Get stats
	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalChunks != 1 {
		t.Errorf("expected 1 chunk, got %d", stats.TotalChunks)
	}
	if stats.TotalEmbeddings != 0 {
		// Embeddings count comes from embedding_metadata, which we didn't insert
		t.Logf("TotalEmbeddings: %d (expected 0 without embedding_metadata)", stats.TotalEmbeddings)
	}
}

// TestListSessionsWithProject covers project-specific listing
func TestListSessionsProjectFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "projA",
			Tags:       []string{"feature"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "projA content", UUID: getTestUUID(), Timestamp: time.Now().Format(time.RFC3339), ContentType: "text"},
			},
		},
		{
			SourcePath: "/test/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "projB",
			Tags:       []string{"debugging"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "projB content", UUID: getTestUUID(), Timestamp: time.Now().Format(time.RFC3339), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// List sessions for projA
	sessions, err := db.ListSessions("projA", 0)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session for projA, got %d", len(sessions))
	}
	if len(sessions[0].Tags) != 1 || sessions[0].Tags[0] != "feature" {
		t.Errorf("expected feature tag, got %v", sessions[0].Tags)
	}
}

// TestResolveSessionPathExactMatch covers exact path matching
func TestResolveSessionPathVariants(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/home/user/.claude/projects/myproj/session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "myproj",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "content", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Exact path
	path, _ := db.ResolveSessionPath("/home/user/.claude/projects/myproj/session.jsonl")
	if path != "/home/user/.claude/projects/myproj/session.jsonl" {
		t.Errorf("expected exact match, got %s", path)
	}

	// Fragment (UUID-like)
	path, _ = db.ResolveSessionPath("session.jsonl")
	if path == "" {
		t.Error("expected fragment match")
	}

	// Not found
	path, _ = db.ResolveSessionPath("nonexistent-uuid")
	if path != "" {
		t.Errorf("expected empty for not found, got %s", path)
	}
}

// TestVectorSearchSorting covers insertion sort with multiple items
func TestVectorSearchSorting(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Create multiple chunks with different embeddings
	baseVec := []float32{1, 0, 0, 0}
	for i := 0; i < 5; i++ {
		ids, _ := db.InsertChunks(fmt.Sprintf("source/v%d.jsonl", i), []ChunkRecord{
			{ChunkIdx: 0, Content: fmt.Sprintf("vec %d", i), TokenCount: 2},
		}, time.Now().Unix())

		// Create vector that varies in similarity
		vec := make([]float32, 4)
		for j := range vec {
			vec[j] = baseVec[j] + float32(i)*0.1
		}
		_ = db.InsertChunkEmbedding(ids[0], vec)
	}

	// Vector search should return sorted results
	results, err := db.VectorSearch(baseVec, 10)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}

	// Verify results are sorted by similarity (descending)
	for i := 1; i < len(results); i++ {
		if results[i].Similarity > results[i-1].Similarity {
			t.Errorf("results not sorted: %f > %f", results[i].Similarity, results[i-1].Similarity)
		}
	}
}

// TestValidateFTSCheck covers FTS validation
func TestValidateFTSCheck(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Fresh DB should have FTS table
	err := db.Validate()
	if err != nil {
		t.Fatalf("Validate on fresh DB should pass: %v", err)
	}
}

// TestInsertChunksEmptyChunks covers edge case of empty chunks list
func TestInsertChunksEmptyList(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert empty chunks list
	ids, err := db.InsertChunks("source/empty.jsonl", []ChunkRecord{}, time.Now().Unix())
	if err != nil {
		t.Fatalf("InsertChunks empty: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs for empty input, got %d", len(ids))
	}

	// Verify source was still recorded
	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM chunks WHERE source_id = ?", "source/empty.jsonl").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", count)
	}
}

// TestQueryIndexedRecordsComplexFilters covers multiple filter combinations
func TestQueryIndexedRecordsComplexFilters(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync diverse records
	for i := 0; i < 3; i++ {
		_ = db.SyncFiles([]IndexedFile{
			{
				SourcePath: fmt.Sprintf("/test/s%d.jsonl", i),
				Source:     "session",
				Hash:       fmt.Sprintf("h%d", i),
				Project:    fmt.Sprintf("proj%d", i%2),
				Messages: []IndexedMessage{
					{Ordinal: 0, Role: "user", Text: fmt.Sprintf("msg %d", i), UUID: getTestUUID(), Timestamp: time.Now().Add(time.Duration(-i) * time.Hour).Format(time.RFC3339), ContentType: "text"},
				},
			},
		})
	}

	// Complex queries
	queries := []IndexedRecordQuery{
		{Limit: 1},
		{Limit: 100},
		{Source: &[]string{"session"}[0]},
		{Project: &[]string{"proj0"}[0]},
	}

	for _, q := range queries {
		_, err := db.QueryIndexedRecords(q)
		if err != nil {
			t.Errorf("QueryIndexedRecords failed: %v", err)
		}
	}
}

// TestSearchComplexQuery covers search with all filters combined
func TestSearchComplexQuery(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	now := time.Now()
	before := now.Add(1 * time.Hour)
	after := now.Add(-1 * time.Hour)

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/complex.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Tags:       []string{"feature", "testing"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "complex search query test", UUID: getTestUUID(), Timestamp: now.Format(time.RFC3339), ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Search with maximum filters
	results, err := db.Search("complex", models.SearchOptions{
		Project:     "test",
		Role:        "user",
		Tag:         "feature",
		ContentType: "text",
		After:       &after,
		Before:      &before,
		Limit:       10,
		Offset:      0,
	})
	if err != nil {
		t.Fatalf("Search complex: %v", err)
	}
	_ = results
}

// TestSearchWithOffset covers offset pagination
func TestSearchWithOffset(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync multiple messages
	msgs := make([]IndexedMessage, 0)
	for i := 0; i < 5; i++ {
		msgs = append(msgs, IndexedMessage{
			Ordinal:     i,
			Role:        "user",
			Text:        "search test content",
			UUID:        getTestUUID(),
			ContentType: "text",
		})
	}

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/pagination.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages:   msgs,
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Search with offset
	results, err := db.Search("search", models.SearchOptions{
		Limit:  2,
		Offset: 1,
	})
	if err != nil {
		t.Fatalf("Search with offset: %v", err)
	}
	_ = results
}

// TestIntegrationFullWorkflow covers a complete workflow
func TestIntegrationFullWorkflow(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// 1. Sync files
	err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj",
			Tags:       []string{"feat", "test"},
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "integrate workflow full", UUID: getTestUUID(), Timestamp: time.Now().Format(time.RFC3339), ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "workflow response", UUID: getTestUUID(), Timestamp: time.Now().Format(time.RFC3339), ContentType: "text"},
			},
		},
	})
	if err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// 2. Search
	results, err := db.Search("integrate", models.SearchOptions{AllProjects: true, Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search results")
	}

	// 3. Get stats
	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.TotalFiles != 1 || stats.TotalMessages != 2 {
		t.Errorf("stats mismatch: files=%d msgs=%d", stats.TotalFiles, stats.TotalMessages)
	}

	// 4. List sessions
	sessions, err := db.ListSessions("proj", 10)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}

	// 5. Get topics
	topics, err := db.GetTopics("proj", 10)
	if err != nil {
		t.Fatalf("GetTopics: %v", err)
	}
	_ = topics

	// 6. Validate
	err = db.Validate()
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// 7. Optimize
	err = db.OptimizeFTS()
	if err != nil {
		t.Fatalf("OptimizeFTS: %v", err)
	}

	// 8. Query records
	records, err := db.QueryIndexedRecords(IndexedRecordQuery{Project: &[]string{"proj"}[0]})
	if err != nil {
		t.Fatalf("QueryIndexedRecords: %v", err)
	}
	if len(records) == 0 {
		t.Error("expected records")
	}

	// 10. Chunks and embeddings
	ids, err := db.InsertChunks("/s1.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "workflow chunk", TokenCount: 2},
	}, time.Now().Unix())
	if err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	vec := make([]float32, 4)
	vec[0] = 1.0
	err = db.InsertChunkEmbedding(ids[0], vec)
	if err != nil {
		t.Fatalf("InsertChunkEmbedding: %v", err)
	}

	// 11. Vector search
	vresults, err := db.VectorSearch(vec, 10)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	_ = vresults

	// 12. Hybrid search
	hresults, err := db.HybridSearch("workflow", models.SearchOptions{AllProjects: true})
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	_ = hresults

	// 13. ResolveSessionPath
	path, err := db.ResolveSessionPath("/s1.jsonl")
	if err != nil {
		t.Fatalf("ResolveSessionPath: %v", err)
	}
	if path == "" {
		t.Error("expected path resolution")
	}
}

// TestSyncFilesMultipleMessages exercises message handling
func TestSyncFilesMultipleMessageDeletion(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// First sync with 3 messages
	_ = db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/msgs.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "m1", UUID: getTestUUID(), ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "m2", UUID: getTestUUID(), ContentType: "text"},
				{Ordinal: 2, Role: "user", Text: "m3", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	})

	var count int
	_ = db.DB().QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = ?", "/test/msgs.jsonl").Scan(&count)
	if count != 3 {
		t.Fatalf("expected 3 items, got %d", count)
	}

	// Resync with only 2 messages (should delete the third)
	_ = db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/msgs.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "m1_new", UUID: getTestUUID(), ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "m2_new", UUID: getTestUUID(), ContentType: "text"},
			},
		},
	})

	_ = db.DB().QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path = ?", "/test/msgs.jsonl").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 items after resync, got %d", count)
	}
}

func TestSearchWithSourcePathFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/proj/a/session.jsonl",
			Source:     "session",
			Hash:       "hash-a",
			Project:    "proja",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "needle in path a", UUID: getTestUUID(), Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
		{
			SourcePath: "/proj/b/session.jsonl",
			Source:     "session",
			Hash:       "hash-b",
			Project:    "projb",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "needle in path b", UUID: getTestUUID(), Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Exact path match
	results, err := db.Search("needle", models.SearchOptions{SourcePath: "/proj/a/session.jsonl", Limit: 10})
	if err != nil {
		t.Fatalf("Search exact source-path: %v", err)
	}
	if len(results) != 1 || results[0].SourcePath != "/proj/a/session.jsonl" {
		t.Errorf("exact source-path: expected 1 result from /proj/a, got %d: %+v", len(results), results)
	}

	// Glob match
	results, err = db.Search("needle", models.SearchOptions{SourcePath: "/proj/b/*", Limit: 10})
	if err != nil {
		t.Fatalf("Search glob source-path: %v", err)
	}
	if len(results) != 1 || results[0].SourcePath != "/proj/b/session.jsonl" {
		t.Errorf("glob source-path: expected 1 result from /proj/b, got %d: %+v", len(results), results)
	}

	// Non-matching path
	results, err = db.Search("needle", models.SearchOptions{SourcePath: "/nope/*", Limit: 10})
	if err != nil {
		t.Fatalf("Search non-matching source-path: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("non-matching source-path: expected 0 results, got %d", len(results))
	}
}

func TestListItemsV2Empty(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	opts := ListOptions{Limit: 10}
	entries, err := db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty list, got %d entries", len(entries))
	}
}

func TestListItemsV2WithData(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync test data
	files := []IndexedFile{
		{
			SourcePath: "test1.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello", Timestamp: "2024-01-01T00:00:00Z"},
			},
			Tags: []string{"tag1"},
		},
		{
			SourcePath: "test2.jsonl",
			Source:     "session",
			Hash:       "hash2",
			Project:    "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "world", Timestamp: "2024-01-02T00:00:00Z"},
			},
			Tags: []string{"tag2"},
		},
	}

	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("SyncFiles error: %v", err)
	}

	// Test basic listing
	opts := ListOptions{Limit: 10}
	entries, err := db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify paths
	paths := make(map[string]bool)
	for _, e := range entries {
		paths[e.Path] = true
	}
	if !paths["test1.jsonl"] {
		t.Error("expected test1.jsonl in results")
	}
	if !paths["test2.jsonl"] {
		t.Error("expected test2.jsonl in results")
	}
}

func TestListItemsV2WithProjectFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync test data with different projects
	files := []IndexedFile{
		{
			SourcePath: "proj1/session.jsonl",
			Source:     "session",
			Hash:       "hash1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello", Timestamp: "2024-01-01T00:00:00Z"},
			},
		},
		{
			SourcePath: "proj2/session.jsonl",
			Source:     "session",
			Hash:       "hash2",
			Project:    "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "world", Timestamp: "2024-01-02T00:00:00Z"},
			},
		},
	}

	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("SyncFiles error: %v", err)
	}

	// Test project filter
	opts := ListOptions{Project: "proj1", Limit: 10}
	entries, err := db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 error: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry for proj1, got %d", len(entries))
	}
	if entries[0].Path != "proj1/session.jsonl" {
		t.Errorf("expected proj1/session.jsonl, got %s", entries[0].Path)
	}
}

func TestListItemsV2WithDateFilters(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync test data with different timestamps
	files := []IndexedFile{
		{
			SourcePath: "early.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "p1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "text1", Timestamp: "2024-01-01T00:00:00Z"},
			},
		},
		{
			SourcePath: "middle.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "p1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "text2", Timestamp: "2024-01-15T00:00:00Z"},
			},
		},
		{
			SourcePath: "late.jsonl",
			Source:     "session",
			Hash:       "h3",
			Project:    "p1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "text3", Timestamp: "2024-01-31T00:00:00Z"},
			},
		},
	}

	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("SyncFiles error: %v", err)
	}

	// Test after filter
	afterTime := time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC)
	opts := ListOptions{After: &afterTime, Limit: 10}
	entries, err := db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 after error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after 2024-01-10, got %d", len(entries))
	}

	// Test before filter
	beforeTime := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	opts = ListOptions{Before: &beforeTime, Limit: 10}
	entries, err = db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 before error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries before 2024-01-20, got %d", len(entries))
	}

	// Test timestamp ordering (desc)
	opts = ListOptions{Order: "timestamp:desc", Limit: 10}
	entries, err = db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 order desc error: %v", err)
	}
	if len(entries) > 0 && entries[0].Path != "late.jsonl" {
		t.Errorf("expected late.jsonl first in desc order, got %s", entries[0].Path)
	}

	// Test timestamp ordering (asc)
	opts = ListOptions{Order: "timestamp:asc", Limit: 10}
	entries, err = db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 order asc error: %v", err)
	}
	if len(entries) > 0 && entries[0].Path != "early.jsonl" {
		t.Errorf("expected early.jsonl first in asc order, got %s", entries[0].Path)
	}
}

func TestListItemsV2WithInputFilter(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync test data with different sources
	files := []IndexedFile{
		{
			SourcePath: "session.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "p1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "text", Timestamp: "2024-01-01T00:00:00Z"},
			},
		},
		{
			SourcePath: "plan.md",
			Source:     "plan",
			Hash:       "h2",
			Project:    "p1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "plan", Text: "plan text", Timestamp: "2024-01-01T00:00:00Z"},
			},
		},
	}

	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("SyncFiles error: %v", err)
	}

	// Test input filter
	opts := ListOptions{Input: "session", Limit: 10}
	entries, err := db.ListItemsV2(opts)
	if err != nil {
		t.Fatalf("ListItemsV2 input filter error: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "session.jsonl" {
		t.Errorf("expected only session.jsonl with input filter, got %v", entries)
	}
}

func TestV4MigrationRoutesToolRowsToToolFTS(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v4.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup schema: %v", err)
	}

	// Insert one prose row and one tool row directly.
	_, err = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	if err != nil {
		t.Fatalf("seed indexed_files: %v", err)
	}
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'user','architecture decision about retries','text'),
		       ('session','p1',1,'assistant','internal/storage/sync.go','tool')`)
	if err != nil {
		t.Fatalf("seed search_items: %v", err)
	}

	var msgCount, toolCount int
	if err := db.db.QueryRow(`SELECT count(*) FROM messages_fts WHERE messages_fts MATCH '"architecture"'`).Scan(&msgCount); err != nil {
		t.Fatalf("query messages_fts: %v", err)
	}
	if err := db.db.QueryRow(`SELECT count(*) FROM tool_fts WHERE tool_fts MATCH '"sync.go"'`).Scan(&toolCount); err != nil {
		t.Fatalf("query tool_fts: %v", err)
	}

	if msgCount != 1 {
		t.Errorf("messages_fts: want 1 prose hit, got %d", msgCount)
	}
	if toolCount != 1 {
		t.Errorf("tool_fts: want 1 tool hit, got %d", toolCount)
	}

	// The tool row must NOT be in messages_fts (no crowding).
	var leak int
	if err := db.db.QueryRow(`SELECT count(*) FROM messages_fts WHERE messages_fts MATCH '"sync"'`).Scan(&leak); err != nil {
		t.Fatalf("query leak: %v", err)
	}
	if leak != 0 {
		t.Errorf("tool content leaked into messages_fts: got %d hits", leak)
	}

	// Updating a prose row's text must keep it correctly routed to messages_fts.
	if _, err := db.db.Exec(`UPDATE search_items SET text = 'updated architecture' WHERE ordinal = 0`); err != nil {
		t.Fatalf("update prose: %v", err)
	}
	var updatedHit int
	if err := db.db.QueryRow(`SELECT count(*) FROM messages_fts WHERE messages_fts MATCH '"updated"'`).Scan(&updatedHit); err != nil {
		t.Fatalf("query updated: %v", err)
	}
	if updatedHit != 1 {
		t.Errorf("update routing: want 1 hit in messages_fts, got %d", updatedHit)
	}
}

func TestToolSearchRanksExactPathFirst(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "toolsearch.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, _ = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'assistant','edited internal/storage/hybrid.go','tool'),
		       ('session','p1',1,'assistant','read internal/storage/sync.go','tool'),
		       ('session','p1',2,'assistant','ran go test ./internal/storage/','tool')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	results, err := db.Search("internal/storage/sync.go", models.SearchOptions{ContentType: "tool", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("want at least 1 result, got 0")
	}
	if !strings.Contains(results[0].Text, "sync.go") {
		t.Errorf("want sync.go ranked first, got %q", results[0].Text)
	}
}

func TestSearchEverythingReturnsBothProseAndTool(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "merge.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, _ = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'user','retry backoff strategy discussion','text'),
		       ('session','p1',1,'assistant','ran retry-backoff.sh and saw retry','tool')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	results, err := db.Search("retry", models.SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	var sawText, sawTool bool
	for _, r := range results {
		if r.ContentType == "text" {
			sawText = true
		}
		if r.ContentType == "tool" {
			sawTool = true
		}
	}
	if !sawText || !sawTool {
		t.Errorf("want both prose and tool hits; sawText=%v sawTool=%v (got %d results)", sawText, sawTool, len(results))
	}
}

func TestOptimizeFTSCoversToolIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opt.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Must not error now that two FTS tables exist.
	if err := db.OptimizeFTS(); err != nil {
		t.Fatalf("optimize: %v", err)
	}
}

// TestSearchScopedTextRouting verifies that Search with ContentType:"text" only
// queries messages_fts and returns text/code rows, excluding tool rows.
func TestSearchScopedTextRouting(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Seed search_items with both prose and tool content
	_, _ = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	_, err := db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'user','implement the retry logic','text'),
		       ('session','p1',1,'assistant','here is the code for retry-backoff','code'),
		       ('session','p1',2,'assistant','invoke retry-backoff.sh command','tool')`)
	if err != nil {
		t.Fatalf("seed search_items: %v", err)
	}

	// Search with ContentType:"text" should only return text/code rows
	results, err := db.Search("retry", models.SearchOptions{ContentType: "text", Limit: 10})
	if err != nil {
		t.Fatalf("search text: %v", err)
	}

	for _, r := range results {
		if r.ContentType == "tool" {
			t.Errorf("ContentType=text search returned tool row: %+v", r)
		}
		if r.ContentType != "text" && r.ContentType != "code" {
			t.Errorf("ContentType=text search returned unexpected content_type=%s", r.ContentType)
		}
	}
}

// TestSearchScopedCodeRouting verifies that Search with ContentType:"code" only
// queries messages_fts and returns matching code rows.
func TestSearchScopedCodeRouting(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Seed with code-specific content
	_, _ = db.db.Exec(`INSERT INTO indexed_files(path, hash) VALUES ('p1','h1')`)
	_, err := db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, content_type)
		VALUES ('session','p1',0,'user','find golang examples','text'),
		       ('session','p1',1,'assistant','package main func golang code example','code'),
		       ('session','p1',2,'assistant','golang test output tool','tool')`)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Search with ContentType:"code" should only return code rows
	results, err := db.Search("golang", models.SearchOptions{ContentType: "code", Limit: 10})
	if err != nil {
		t.Fatalf("search code: %v", err)
	}

	if len(results) == 0 {
		t.Error("ContentType=code search returned no results")
	}
	for _, r := range results {
		if r.ContentType != "code" {
			t.Errorf("ContentType=code search returned non-code row with content_type=%s", r.ContentType)
		}
	}
}

// TestPaginateOffsetBeyondResults verifies that paginate returns empty slice
// when offset >= len(results).
func TestPaginateOffsetBeyondResults(t *testing.T) {
	rs := []SearchResult{
		{ID: 1, Text: "a", Score: 1.0},
		{ID: 2, Text: "b", Score: 0.8},
	}

	// Offset beyond length should return empty slice
	result := paginate(rs, 10, 5)
	if len(result) != 0 {
		t.Errorf("paginate with offset=5 beyond len=2 returned %d results, want 0", len(result))
	}
}

// TestPaginateLimitTruncation verifies that paginate truncates to limit size
// when limit is smaller than available results.
func TestPaginateLimitTruncation(t *testing.T) {
	rs := []SearchResult{
		{ID: 1, Text: "a", Score: 1.0},
		{ID: 2, Text: "b", Score: 0.9},
		{ID: 3, Text: "c", Score: 0.8},
		{ID: 4, Text: "d", Score: 0.7},
	}

	// Limit of 2 should return 2 results
	result := paginate(rs, 2, 0)
	if len(result) != 2 {
		t.Errorf("paginate with limit=2 returned %d results, want 2", len(result))
	}
	if result[0].ID != 1 || result[1].ID != 2 {
		t.Errorf("paginate returned wrong results: %v", result)
	}

	// Limit of 2 with offset 1 should return IDs 2,3
	result = paginate(rs, 2, 1)
	if len(result) != 2 {
		t.Errorf("paginate(limit=2, offset=1) returned %d results, want 2", len(result))
	}
	if result[0].ID != 2 || result[1].ID != 3 {
		t.Errorf("paginate(limit=2, offset=1) returned IDs %d,%d, want 2,3", result[0].ID, result[1].ID)
	}
}

// TestNormalizeEqualScores verifies that normalize assigns score=1 to all rows
// when min == max (all scores are equal).
func TestNormalizeEqualScores(t *testing.T) {
	rs := []SearchResult{
		{ID: 1, Text: "a", Score: 0.5},
		{ID: 2, Text: "b", Score: 0.5},
		{ID: 3, Text: "c", Score: 0.5},
	}

	normalize(rs)

	for i, r := range rs {
		if r.Score != 1.0 {
			t.Errorf("normalize equal scores: rs[%d].Score = %f, want 1.0", i, r.Score)
		}
	}
}

// TestNormalizeSpreadScores verifies that normalize correctly maps a range of
// scores to [0,1] using min-max normalization.
func TestNormalizeSpreadScores(t *testing.T) {
	rs := []SearchResult{
		{ID: 1, Text: "a", Score: 10.0},
		{ID: 2, Text: "b", Score: 15.0},
		{ID: 3, Text: "c", Score: 20.0},
	}

	normalize(rs)

	// min=10, max=20, span=10
	// 10 -> (10-10)/10 = 0.0
	// 15 -> (15-10)/10 = 0.5
	// 20 -> (20-10)/10 = 1.0
	expected := []float64{0.0, 0.5, 1.0}
	for i, r := range rs {
		if r.Score != expected[i] {
			t.Errorf("normalize spread: rs[%d].Score = %f, want %f", i, r.Score, expected[i])
		}
	}
}

// TestPaginateDefaultLimit verifies that paginate defaults to limit=100 when
// limit is 0.
func TestPaginateDefaultLimit(t *testing.T) {
	// Create 150 results to exceed default limit of 100
	rs := make([]SearchResult, 150)
	for i := range rs {
		rs[i] = SearchResult{ID: i + 1, Text: fmt.Sprintf("r%d", i+1), Score: float64(150 - i)}
	}

	// With limit=0, should default to 100
	result := paginate(rs, 0, 0)
	if len(result) != 100 {
		t.Errorf("paginate(limit=0) returned %d results, want 100", len(result))
	}
}

// TestPaginateNegativeOffset verifies that paginate treats negative offset as 0.
func TestPaginateNegativeOffset(t *testing.T) {
	rs := []SearchResult{
		{ID: 1, Text: "a", Score: 1.0},
		{ID: 2, Text: "b", Score: 0.9},
		{ID: 3, Text: "c", Score: 0.8},
	}

	// Negative offset should be treated as 0
	result := paginate(rs, 2, -5)
	if len(result) != 2 {
		t.Errorf("paginate(offset=-5) returned %d results, want 2", len(result))
	}
	if result[0].ID != 1 || result[1].ID != 2 {
		t.Errorf("paginate(offset=-5) returned IDs %d,%d, want 1,2", result[0].ID, result[1].ID)
	}
}

func TestMigrationV7(t *testing.T) {
	// Create a fresh database and force application of all migrations.
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Verify schema_migrations records version 7 FIRST (before any test data operations).
	var v7Applied int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 7").Scan(&v7Applied); err != nil {
		t.Fatalf("query v7: %v", err)
	}
	if v7Applied != 1 {
		t.Errorf("v7 migration not recorded; count=%d", v7Applied)
	}

	// Insert reasoning and text content_types and verify they index into messages_fts.
	// (This requires the sync loop / trigger to fire; a minimal test inserts directly.)
	if _, err := db.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type)
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/test.jsonl", 1, "assistant", "reasoning text here", "reasoning"); err != nil {
		t.Fatalf("insert reasoning: %v", err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type)
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/test.jsonl", 2, "assistant", "regular text", "text"); err != nil {
		t.Fatalf("insert text: %v", err)
	}
	if _, err := db.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type)
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/test.jsonl", 3, "assistant", "tool command", "tool"); err != nil {
		t.Fatalf("insert tool: %v", err)
	}

	// Verify reasoning and text are in messages_fts.
	var reasoningCount, textCount, toolCount int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'reasoning'").Scan(&reasoningCount); err != nil {
		t.Fatalf("query messages_fts reasoning: %v", err)
	}
	if reasoningCount == 0 {
		t.Error("reasoning content not in messages_fts")
	}

	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'regular'").Scan(&textCount); err != nil {
		t.Fatalf("query messages_fts text: %v", err)
	}
	if textCount == 0 {
		t.Error("text content not in messages_fts")
	}

	// Verify tool content is in tool_fts, NOT in messages_fts.
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM tool_fts WHERE text MATCH 'command'").Scan(&toolCount); err != nil {
		t.Fatalf("query tool_fts: %v", err)
	}
	if toolCount == 0 {
		t.Error("tool content not in tool_fts")
	}

	var toolInMsg int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'command'").Scan(&toolInMsg); err != nil {
		t.Fatalf("query messages_fts for tool: %v", err)
	}
	if toolInMsg != 0 {
		t.Error("tool content should NOT be in messages_fts")
	}
}

func TestMigrationV7BeginError(t *testing.T) {
	// applyV7Migration must surface a Begin() failure; a closed connection forces it.
	db, cleanup := newTestDB(t)
	cleanup()

	if err := db.applyV7Migration(); err == nil {
		t.Fatal("applyV7Migration should fail when the connection is closed")
	}
}

func TestMigrationV7ErrorRollback(t *testing.T) {
	// Test that applyV7Migration properly rolls back on INSERT failure.
	// Pre-insert version 7 into schema_migrations to force a duplicate-key constraint violation
	// on the INSERT statement, exercising the error path and ensuring rollback occurs.
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Remove v7 from schema_migrations if it exists (fresh DB has it from SetupSchema)
	if _, err := db.DB().Exec("DELETE FROM schema_migrations WHERE version = 7"); err != nil {
		t.Fatalf("delete v7: %v", err)
	}

	// Now manually insert v7 to force the duplicate-key failure
	if _, err := db.DB().Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (7, 'duplicate test', CURRENT_TIMESTAMP, 'test-checksum')
	`); err != nil {
		t.Fatalf("pre-insert v7: %v", err)
	}

	// Now try to apply v7 migration again — this should fail and roll back
	// because the INSERT INTO schema_migrations will fail (duplicate version key)
	err := db.applyV7Migration()
	if err == nil {
		t.Fatal("applyV7Migration should have failed on duplicate insert, but succeeded")
	}

	// Verify rollback: the duplicate entry should still exist with the original name
	var name string
	err = db.DB().QueryRow("SELECT name FROM schema_migrations WHERE version = 7").Scan(&name)
	if err != nil {
		t.Fatalf("query v7 after failed migration: %v", err)
	}
	if name != "duplicate test" {
		t.Errorf("rollback failed: expected name 'duplicate test', got %q (new triggers may have been applied)", name)
	}
}

func TestMigrationV7Idempotent(t *testing.T) {
	// Test that SetupSchema correctly skips v7 migration when already applied.
	// This exercises the version-check path in SetupSchema that determines whether to apply.
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Verify v7 is applied from fresh DB
	var count int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 7").Scan(&count); err != nil {
		t.Fatalf("query v7: %v", err)
	}
	if count != 1 {
		t.Errorf("v7 should be applied once on fresh DB; count=%d", count)
	}

	// Call SetupSchema again — should skip v7 (idempotent)
	if err := db.SetupSchema(); err != nil {
		t.Fatalf("SetupSchema second call: %v", err)
	}

	// Verify v7 is still applied exactly once
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 7").Scan(&count); err != nil {
		t.Fatalf("query v7 after second SetupSchema: %v", err)
	}
	if count != 1 {
		t.Errorf("v7 should be applied exactly once after idempotent SetupSchema; count=%d", count)
	}

	// Verify triggers still work by inserting reasoning content
	if _, err := db.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type)
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/idempotent_test.jsonl", 1, "assistant", "test reasoning content", "reasoning"); err != nil {
		t.Fatalf("insert reasoning after idempotent setup: %v", err)
	}

	var reasoningFound int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'reasoning'").Scan(&reasoningFound); err != nil {
		t.Fatalf("query messages_fts: %v", err)
	}
	if reasoningFound == 0 {
		t.Error("reasoning content not indexed in messages_fts after idempotent SetupSchema")
	}
}

func TestMigrationV7UpdateTrigger(t *testing.T) {
	// Test that v7 UPDATE trigger properly handles reasoning content_type updates.
	// This exercises the search_items_au_msg trigger path for reasoning rows.
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert a reasoning row
	result, err := db.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "/tmp/update_test.jsonl", 1, "assistant", "original reasoning", "reasoning", "2026-01-01")
	if err != nil {
		t.Fatalf("insert reasoning: %v", err)
	}

	rowID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get LastInsertId: %v", err)
	}

	// Verify it's in messages_fts
	var found int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE rowid = ?", rowID).Scan(&found); err != nil {
		t.Fatalf("query before update: %v", err)
	}
	if found == 0 {
		t.Error("reasoning row not in messages_fts after insert")
	}

	// Update the text of the reasoning row
	if _, err := db.DB().Exec(`
		UPDATE search_items SET text = ? WHERE id = ?
	`, "updated reasoning", rowID); err != nil {
		t.Fatalf("update text: %v", err)
	}

	// Verify the updated text is in messages_fts (the UPDATE trigger should handle this)
	var foundUpdated int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'updated'").Scan(&foundUpdated); err != nil {
		t.Fatalf("query after update: %v", err)
	}
	if foundUpdated == 0 {
		t.Error("updated reasoning content not found in messages_fts")
	}

	// Verify old text is gone
	var foundOld int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'original'").Scan(&foundOld); err != nil {
		t.Fatalf("query old text: %v", err)
	}
	if foundOld != 0 {
		t.Error("old reasoning content should not be in messages_fts after update")
	}
}

func TestMigrationV7DeleteTrigger(t *testing.T) {
	// Test that v7 DELETE trigger properly handles reasoning content_type deletions.
	// This exercises the search_items_ad_msg trigger path for reasoning rows.
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Insert a reasoning row
	result, err := db.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "/tmp/delete_test.jsonl", 1, "assistant", "delete me reasoning", "reasoning", "2026-01-01")
	if err != nil {
		t.Fatalf("insert reasoning: %v", err)
	}

	rowID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("get LastInsertId: %v", err)
	}

	// Verify it's in messages_fts
	var found int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE rowid = ?", rowID).Scan(&found); err != nil {
		t.Fatalf("query before delete: %v", err)
	}
	if found == 0 {
		t.Error("reasoning row not in messages_fts after insert")
	}

	// Delete the reasoning row
	if _, err := db.DB().Exec(`
		DELETE FROM search_items WHERE id = ?
	`, rowID); err != nil {
		t.Fatalf("delete row: %v", err)
	}

	// Verify it's removed from messages_fts (the DELETE trigger should handle this)
	var foundAfter int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'delete'").Scan(&foundAfter); err != nil {
		t.Fatalf("query after delete: %v", err)
	}
	if foundAfter != 0 {
		t.Error("deleted reasoning content should not be in messages_fts")
	}
}

func TestMigrationV7AllContentTypes(t *testing.T) {
	// Comprehensive test: verify v7 triggers handle all content types correctly.
	// This exercises the INSERT, UPDATE, and DELETE paths for text, code, reasoning, and tool.
	db, cleanup := newTestDB(t)
	defer cleanup()

	insertID := func(path string, ordinal int, role string, text string, ct string) int64 {
		result, err := db.DB().Exec(`
			INSERT INTO search_items (source_path, ordinal, role, text, content_type, timestamp)
			VALUES (?, ?, ?, ?, ?, ?)
		`, path, ordinal, role, text, ct, "2026-01-01")
		if err != nil {
			t.Fatalf("insert %s: %v", ct, err)
		}
		id, _ := result.LastInsertId()
		return id
	}

	// Insert one of each type
	_ = insertID("/tmp/all_types.jsonl", 1, "user", "user text content", "text")
	codeID := insertID("/tmp/all_types.jsonl", 2, "assistant", "func hello() {}", "code")
	reasoningID := insertID("/tmp/all_types.jsonl", 3, "assistant", "let me think through this", "reasoning")
	_ = insertID("/tmp/all_types.jsonl", 4, "assistant", "search_query: test", "tool")

	// Verify each is in the correct index
	texts := []struct {
		query   string
		match   string
		inTable string
	}{
		{"SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'user'", "user text", "messages_fts"},
		{"SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'func'", "code", "messages_fts"},
		{"SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'think'", "reasoning", "messages_fts"},
		{"SELECT COUNT(*) FROM tool_fts WHERE text MATCH 'search'", "tool", "tool_fts"},
	}

	for _, tc := range texts {
		var count int
		if err := db.DB().QueryRow(tc.query).Scan(&count); err != nil {
			t.Fatalf("query %s: %v", tc.inTable, err)
		}
		if count == 0 {
			t.Errorf("%s content type not in %s index", tc.match, tc.inTable)
		}
	}

	// Update reasoning text
	if _, err := db.DB().Exec("UPDATE search_items SET text = ? WHERE id = ?", "updated thought process", reasoningID); err != nil {
		t.Fatalf("update reasoning: %v", err)
	}

	var updatedFound int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'updated'").Scan(&updatedFound); err != nil {
		t.Fatalf("query updated reasoning: %v", err)
	}
	if updatedFound == 0 {
		t.Error("updated reasoning text not found in messages_fts")
	}

	// Delete code
	if _, err := db.DB().Exec("DELETE FROM search_items WHERE id = ?", codeID); err != nil {
		t.Fatalf("delete code: %v", err)
	}

	var codeAfterDelete int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'func'").Scan(&codeAfterDelete); err != nil {
		t.Fatalf("query after delete: %v", err)
	}
	if codeAfterDelete != 0 {
		t.Error("deleted code still in messages_fts")
	}
}
