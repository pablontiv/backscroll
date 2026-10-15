package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStalePathsReturnsPreV8Files(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert a v8 row (extraction_version=1)
	_, _ = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/p/rich.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', 'u1', 'proj', 'text', 1, 0)`)

	// Insert a pre-v8 row (extraction_version IS NULL)
	_, _ = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/p/legacy.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', NULL, 'proj', 'text', NULL, 0)`)

	// Insert a v7 row (extraction_version=0, hypothetical older version)
	_, _ = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/p/stale.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', 'u2', 'proj', 'text', 0, 0)`)

	// Non-session source should not appear
	_, _ = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('plan', '/p/plan.md', 0, 'user', 'text', '2026-01-01T00:00:00Z', NULL, 'proj', 'text', NULL, 0)`)

	paths, err := db.StalePaths(CurrentExtractionVersion)
	if err != nil {
		t.Fatalf("stale paths: %v", err)
	}

	if len(paths) != 3 {
		t.Fatalf("want 3 stale paths (v1 + legacy + v0), got %d: %v", len(paths), paths)
	}
	if paths[0] != "/p/legacy.jsonl" || paths[1] != "/p/rich.jsonl" || paths[2] != "/p/stale.jsonl" {
		t.Errorf("stale paths = %v, want [/p/legacy.jsonl /p/rich.jsonl /p/stale.jsonl]", paths)
	}

	// Non-session source must not appear
	for _, p := range paths {
		if p == "/p/plan.md" {
			t.Error("plan sources must not appear in stale paths")
		}
	}
}

func TestStalePathsOrderedByLastIndexed(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert indexed_files with different timestamps
	_, _ = db.db.Exec(`INSERT INTO indexed_files (path, hash, last_indexed)
		VALUES ('/p/newer.jsonl', 'h1', '2026-01-02T00:00:00Z')`)
	_, _ = db.db.Exec(`INSERT INTO indexed_files (path, hash, last_indexed)
		VALUES ('/p/older.jsonl', 'h2', '2026-01-01T00:00:00Z')`)

	// Insert search_items for both (both stale)
	_, _ = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/p/newer.jsonl', 0, 'user', 'text', '2026-01-02T00:00:00Z', NULL, 'proj', 'text', NULL, 0)`)
	_, _ = db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/p/older.jsonl', 0, 'user', 'text', '2026-01-01T00:00:00Z', NULL, 'proj', 'text', NULL, 0)`)

	paths, err := db.StalePaths(CurrentExtractionVersion)
	if err != nil {
		t.Fatalf("stale paths: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("want 2 stale paths, got %d", len(paths))
	}

	// Older should come first (FIFO draining)
	if paths[0] != "/p/older.jsonl" || paths[1] != "/p/newer.jsonl" {
		t.Errorf("stale paths = %v, want [/p/older.jsonl /p/newer.jsonl] (ascending last_indexed)", paths)
	}
}

func TestOpenReadOnlyDBNotFound(t *testing.T) {
	// OpenReadOnly should fail fast if DB file doesn't exist
	_, err := OpenReadOnly("/nonexistent/path/to/db.db")
	if err == nil {
		t.Error("expected error for nonexistent database, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}
