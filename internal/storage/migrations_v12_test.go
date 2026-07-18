package storage

import (
	"path/filepath"
	"testing"
)

func TestV12MigrationAddsAnnotations(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// annotations exists with expected columns
	if _, err := db.db.Exec(`INSERT INTO annotations
		(item_uuid, source_path, ordinal, kind, label, source, created_at)
		VALUES ('u1', '/p/s.jsonl', 0, 'correction', 'fixable', 'agent', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert annotations: %v", err)
	}

	// UNIQUE(source_path, ordinal, kind) enforced
	if _, err := db.db.Exec(`INSERT INTO annotations
		(item_uuid, source_path, ordinal, kind, label, source, created_at)
		VALUES ('u2', '/p/s.jsonl', 0, 'correction', 'duplicate', 'agent', '2026-01-01T00:00:01Z')`); err == nil {
		t.Fatal("expected UNIQUE(source_path, ordinal, kind) violation")
	}

	// INSERT OR REPLACE replaces (same uuid, ordinal, kind with different label)
	if _, err := db.db.Exec(`INSERT OR REPLACE INTO annotations
		(item_uuid, source_path, ordinal, kind, label, source, created_at)
		VALUES ('u1', '/p/s.jsonl', 0, 'correction', 'false_positive', 'agent', '2026-01-01T00:00:02Z')`); err != nil {
		t.Fatalf("replace annotations: %v", err)
	}

	var label string
	if err := db.db.QueryRow("SELECT label FROM annotations WHERE source_path = '/p/s.jsonl' AND ordinal = 0").Scan(&label); err != nil {
		t.Fatalf("query annotations: %v", err)
	}
	if label != "false_positive" {
		t.Errorf("expected label 'false_positive' after replace, got %q", label)
	}

	// migration recorded
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 12").Scan(&n); err != nil || n != 1 {
		t.Fatalf("v12 not recorded: n=%d err=%v", n, err)
	}
}
