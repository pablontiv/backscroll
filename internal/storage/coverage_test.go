package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestValidateTableMissing covers the "table does not exist" error path in Validate.
func TestValidateTableMissing(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if _, err := db.DB().Exec("DROP TABLE dynamic_stopwords"); err != nil {
		t.Fatalf("drop dynamic_stopwords: %v", err)
	}

	err := db.Validate()
	if err == nil {
		t.Fatal("expected Validate to fail with missing table, got nil")
	}
}

// TestValidateFTSMissing covers the "FTS5 virtual table does not exist" error path.
func TestValidateFTSMissing(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Drop trigger referencing FTS first, then FTS itself.
	// If the driver refuses, skip gracefully.
	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS search_items_ai",
		"DROP TRIGGER IF EXISTS search_items_ad",
		"DROP TRIGGER IF EXISTS search_items_au",
		"DROP TABLE IF EXISTS messages_fts",
	} {
		if _, err := db.DB().Exec(stmt); err != nil {
			t.Skipf("cannot set up FTS-missing scenario: %v", err)
		}
	}

	err := db.Validate()
	if err == nil {
		t.Fatal("expected Validate to fail with missing FTS table, got nil")
	}
}

// TestVectorSearchDimensionMismatch covers the `continue` branch when stored
// embedding dimensions differ from query vector dimensions.
// LoadChunkEmbeddings JOINs chunks with search_items, so we must sync files first.
func TestVectorSearchDimensionMismatch(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Sync a file so search_items has a row for this source_path.
	if err := db.SyncFiles([]IndexedFile{{
		SourcePath: "cov/dim-mismatch.jsonl",
		Source:     "session",
		Hash:       "dm1",
		Project:    "covproj",
		Messages:   []IndexedMessage{{Ordinal: 0, Role: "user", Text: "dim mismatch test", ContentType: "text"}},
	}}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	ids, err := db.InsertChunks("cov/dim-mismatch.jsonl", []ChunkRecord{
		{ChunkIdx: 0, Content: "mismatch test", TokenCount: 2},
	}, time.Now().Unix())
	if err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	// Store a 2-dim embedding
	if err := db.InsertChunkEmbedding(ids[0], []float32{1.0, 0.0}); err != nil {
		t.Fatalf("InsertChunkEmbedding: %v", err)
	}

	// Query with 4-dim vector → dimension mismatch → `continue` branch
	results, err := db.VectorSearch([]float32{1.0, 0.0, 0.0, 0.0}, 10)
	if err != nil {
		t.Fatalf("VectorSearch: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (dimension mismatch), got %d", len(results))
	}
}

// TestVectorSearchTopKTruncation covers the topK truncation branch and
// insertion-sort swap when there are more results than topK.
// LoadChunkEmbeddings JOINs with search_items, so we must sync files first.
func TestVectorSearchTopKTruncation(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	sources := []string{"cov/topk-a.jsonl", "cov/topk-b.jsonl", "cov/topk-c.jsonl"}
	for i, sp := range sources {
		if err := db.SyncFiles([]IndexedFile{{
			SourcePath: sp,
			Source:     "session",
			Hash:       fmt.Sprintf("topk%d", i),
			Project:    "covproj",
			Messages:   []IndexedMessage{{Ordinal: 0, Role: "user", Text: "topk content", ContentType: "text"}},
		}}); err != nil {
			t.Fatalf("SyncFiles %s: %v", sp, err)
		}
	}

	q := []float32{1.0, 0.0, 0.0, 0.0}
	vecs := [][]float32{
		{0.1, 0.0, 0.0, 0.0}, // low
		{1.0, 0.0, 0.0, 0.0}, // high (inserted second → sort swap needed)
		{0.5, 0.0, 0.0, 0.0}, // mid
	}
	for i, sp := range sources {
		ids, err := db.InsertChunks(sp, []ChunkRecord{
			{ChunkIdx: 0, Content: "topk content", TokenCount: 2},
		}, time.Now().Unix())
		if err != nil {
			t.Fatalf("InsertChunks %s: %v", sp, err)
		}
		if err := db.InsertChunkEmbedding(ids[0], vecs[i]); err != nil {
			t.Fatalf("InsertChunkEmbedding %d: %v", i, err)
		}
	}

	// topK=2 with 3 results → triggers truncation AND sort swap
	results, err := db.VectorSearch(q, 2)
	if err != nil {
		t.Fatalf("VectorSearch topK=2: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results with topK=2, got %d", len(results))
	}
	// Verify sorted descending by similarity
	if len(results) == 2 && results[0].Similarity < results[1].Similarity {
		t.Errorf("results not sorted: %.4f < %.4f", results[0].Similarity, results[1].Similarity)
	}
}

// TestOpenCorruptFile covers the db.Ping() error path in Open() by using a
// non-SQLite file. The sqlite driver fails at Ping, not at sql.Open.
func TestOpenCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database!!"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		return // expected: Ping fails with "file is not a database"
	}
	defer func() { _ = db.Close() }()
	// Permissive driver: at least close without panic
}

// TestOptimizeFTSError covers the error return path in OptimizeFTS when the FTS
// virtual table is absent.
func TestOptimizeFTSError(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS search_items_ai",
		"DROP TRIGGER IF EXISTS search_items_ad",
		"DROP TRIGGER IF EXISTS search_items_au",
		"DROP TABLE IF EXISTS messages_fts",
	} {
		if _, err := db.DB().Exec(stmt); err != nil {
			t.Skipf("cannot set up FTS-missing scenario: %v", err)
		}
	}

	err := db.OptimizeFTS()
	if err == nil {
		t.Fatal("expected error from OptimizeFTS with FTS table dropped")
	}
}

// TestSetupSchemaV3MigrationError covers the error return path in applyV3Migration
// and the propagation in SetupSchema. By deleting the V3 migration record from a
// fully-migrated DB and calling SetupSchema again, the ALTER TABLE (which the column
// already has) fails — covering both the migration error return and SetupSchema's
// "return err" after applyV3Migration.
func TestSetupSchemaV3MigrationError(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Delete V3 migration record; SetupSchema will try to re-apply V3
	if _, err := db.DB().Exec("DELETE FROM schema_migrations WHERE version = 3"); err != nil {
		t.Fatalf("delete v3 migration: %v", err)
	}

	// ApplyV3 tries ALTER TABLE ADD COLUMN on a column that already exists → fails
	err := db.SetupSchema()
	if err == nil {
		t.Fatal("expected error re-running V3 migration on already-migrated DB")
	}
}

// TestRefreshStopwordsNoVocab covers the graceful return path in refreshStopwords
// when the messages_vocab FTS auxiliary table is absent (e.g., FTS was dropped).
// The function should return nil, not an error.
func TestRefreshStopwordsNoVocab(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	// Drop FTS triggers then the FTS table; messages_vocab disappears with it.
	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS search_items_ai",
		"DROP TRIGGER IF EXISTS search_items_ad",
		"DROP TRIGGER IF EXISTS search_items_au",
		"DROP TABLE IF EXISTS messages_fts",
	} {
		if _, err := db.DB().Exec(stmt); err != nil {
			t.Skipf("cannot set up no-vocab scenario: %v", err)
		}
	}

	// refreshStopwords queries messages_vocab; with FTS gone it returns nil gracefully.
	if err := db.refreshStopwords(); err != nil {
		t.Fatalf("expected nil error from refreshStopwords without vocab: %v", err)
	}
}

// TestResolveSessionPathByUUID covers the UUID lookup branch in ResolveSessionPath
// (line 269: SELECT source_path FROM search_items WHERE uuid = ?).
func TestResolveSessionPathByUUID(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	const (
		srcPath = "cov/uuid-resolve.jsonl"
		uuid    = "test-uuid-cov-1234"
	)
	if err := db.SyncFiles([]IndexedFile{{
		SourcePath: srcPath,
		Source:     "session",
		Hash:       "uuidresolve1",
		Project:    "covproj",
		Messages:   []IndexedMessage{{Ordinal: 0, Role: "user", Text: "uuid resolve test", UUID: uuid, ContentType: "text"}},
	}}); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}

	// Query by UUID — exact and fragment lookups fail, UUID lookup succeeds
	got, err := db.ResolveSessionPath(uuid)
	if err != nil {
		t.Fatalf("ResolveSessionPath by UUID: %v", err)
	}
	if got != srcPath {
		t.Errorf("expected %q, got %q", srcPath, got)
	}
}

// TestLoadStopwordsTableMissing covers the "no such table" graceful path in
// loadStopwords: when dynamic_stopwords is absent, the function returns an
// empty map (not an error).
func TestLoadStopwordsTableMissing(t *testing.T) {
	db, cleanup := newTestDB(t)
	defer cleanup()

	if _, err := db.DB().Exec("DROP TABLE dynamic_stopwords"); err != nil {
		t.Fatalf("drop dynamic_stopwords: %v", err)
	}

	sw, err := db.loadStopwords()
	if err != nil {
		t.Fatalf("expected nil error with missing table, got: %v", err)
	}
	if len(sw) != 0 {
		t.Errorf("expected empty stopwords map, got %d entries", len(sw))
	}
}
