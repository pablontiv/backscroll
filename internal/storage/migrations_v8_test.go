package storage

import (
	"path/filepath"
	"testing"
)

func TestV8MigrationAddsToolEventsAndColumns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// tool_events exists with expected columns
	if _, err := db.db.Exec(`INSERT INTO tool_events
		(message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
		VALUES ('u1', '/p/s.jsonl', 0, 'Bash', 'go', 1, NULL, 1)`); err != nil {
		t.Fatalf("insert tool_events: %v", err)
	}

	// UNIQUE(source_path, ordinal) enforced
	if _, err := db.db.Exec(`INSERT INTO tool_events
		(message_uuid, source_path, ordinal, tool_name, extraction_version)
		VALUES ('u2', '/p/s.jsonl', 0, 'Read', 1)`); err == nil {
		t.Fatal("expected UNIQUE(source_path, ordinal) violation")
	}

	// new search_items columns accept values
	if _, err := db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/p/s.jsonl', 0, 'user', 'hi', '2026-01-01T00:00:00Z', 'u9', 'proj', 'text', 1, 1)`); err != nil {
		t.Fatalf("insert search_items with v8 columns: %v", err)
	}

	// migration recorded
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 8").Scan(&n); err != nil || n != 1 {
		t.Fatalf("v8 not recorded: n=%d err=%v", n, err)
	}
}
