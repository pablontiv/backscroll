package storage

import (
	"context"
	"os"
	"path/filepath"
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

func TestSetSessionSourceMetadata(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync a file first so we have rows to update
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/session.jsonl",
			Source:     "session",
			Hash:       "abc123",
			Project:    "test",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello", Timestamp: "2024-01-01T00:00:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	err := db.SetSessionSourceMetadata("/test/session.jsonl", SessionSourceMetadata{
		UUID:      "test-uuid",
		SessionID: "test-session",
	})
	if err != nil {
		t.Fatalf("SetSessionSourceMetadata: %v", err)
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

func TestQuerySessionEvents(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync a file to populate session_events
	if err := db.SyncFiles([]IndexedFile{
		{
			SourcePath: "/test/sess.jsonl",
			Source:     "session",
			Hash:       "abc",
			Project:    "proj",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello world", Timestamp: "2024-06-01T10:00:00Z", ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "hi there", Timestamp: "2024-06-01T10:01:00Z", ContentType: "text"},
			},
		},
	}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	events, err := db.QuerySessionEvents(SessionEventQuery{SourcePath: "/test/sess.jsonl"})
	if err != nil {
		t.Fatalf("QuerySessionEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}

	// Filter by role
	userEvents, err := db.QuerySessionEvents(SessionEventQuery{SourcePath: "/test/sess.jsonl", Role: "user"})
	if err != nil {
		t.Fatalf("QuerySessionEvents role filter: %v", err)
	}
	if len(userEvents) != 1 {
		t.Errorf("expected 1 user event, got %d", len(userEvents))
	}

	// Filter by limit
	limited, err := db.QuerySessionEvents(SessionEventQuery{SourcePath: "/test/sess.jsonl", Limit: 1})
	if err != nil {
		t.Fatalf("QuerySessionEvents limit: %v", err)
	}
	if len(limited) != 1 {
		t.Errorf("expected 1 event with limit=1, got %d", len(limited))
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
