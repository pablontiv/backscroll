package storage

import (
	"path/filepath"
	"testing"
)

func TestV10MigrationAddsMessageTemplates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// message_templates exists with expected columns
	if _, err := db.db.Exec(`INSERT INTO message_templates
		(signature, normalization_version, template_text, occurrence_count, first_seen, last_seen)
		VALUES ('sig1', 1, 'error: <*> database locked', 3, '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`); err != nil {
		t.Fatalf("insert message_templates: %v", err)
	}

	var tmplID int64
	if err := db.db.QueryRow("SELECT id FROM message_templates WHERE signature = 'sig1'").Scan(&tmplID); err != nil {
		t.Fatalf("query template id: %v", err)
	}

	// template_matches exists with UNIQUE(source_path, ordinal, template_id)
	if _, err := db.db.Exec(`INSERT INTO template_matches
		(template_id, item_uuid, source_path, ordinal)
		VALUES (?, 'u1', '/p/s.jsonl', 0)`, tmplID); err != nil {
		t.Fatalf("insert template_matches: %v", err)
	}

	// UNIQUE constraint enforced
	if _, err := db.db.Exec(`INSERT INTO template_matches
		(template_id, item_uuid, source_path, ordinal)
		VALUES (?, 'u2', '/p/s.jsonl', 0)`, tmplID); err == nil {
		t.Fatal("expected UNIQUE(source_path, ordinal, template_id) violation")
	}

	// migration recorded
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 10").Scan(&n); err != nil || n != 1 {
		t.Fatalf("v10 not recorded: n=%d err=%v", n, err)
	}
}
