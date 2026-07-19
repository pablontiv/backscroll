package storage

import (
	"path/filepath"
	"testing"
)

func TestV13MigrationAddsSourcePathIndexes(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert template_matches and correction_signals rows
	_, err = db.db.Exec(`
		INSERT INTO message_templates (signature, normalization_version, template_text, occurrence_count)
		VALUES ('sig1', 1, 'error text', 1)
	`)
	if err != nil {
		t.Fatalf("insert template: %v", err)
	}

	var templateID int64
	err = db.db.QueryRow(`SELECT id FROM message_templates WHERE signature = 'sig1'`).Scan(&templateID)
	if err != nil {
		t.Fatalf("get template id: %v", err)
	}

	_, err = db.db.Exec(`
		INSERT INTO template_matches (template_id, item_uuid, source_path, ordinal)
		VALUES (?, 'u1', '/p/s1.jsonl', 0)
	`, templateID)
	if err != nil {
		t.Fatalf("insert template_match: %v", err)
	}

	_, err = db.db.Exec(`
		INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('u2', '/p/s2.jsonl', 1, 'lexicon', 0.8, 0)
	`)
	if err != nil {
		t.Fatalf("insert correction_signal: %v", err)
	}

	// Verify indexes exist via PRAGMA index_list
	rows, err := db.db.Query(`PRAGMA index_list(template_matches)`)
	if err != nil {
		t.Fatalf("pragma index_list template_matches: %v", err)
	}
	defer rows.Close()

	var indexFound bool
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan pragma: %v", err)
		}
		if name == "idx_template_matches_source" {
			indexFound = true
			break
		}
	}
	if !indexFound {
		t.Fatal("idx_template_matches_source not found")
	}

	// Verify correction_signals index
	rows, err = db.db.Query(`PRAGMA index_list(correction_signals)`)
	if err != nil {
		t.Fatalf("pragma index_list correction_signals: %v", err)
	}
	defer rows.Close()

	indexFound = false
	for rows.Next() {
		var seq, unique, partial int
		var name, origin string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan pragma: %v", err)
		}
		if name == "idx_correction_signals_source" {
			indexFound = true
			break
		}
	}
	if !indexFound {
		t.Fatal("idx_correction_signals_source not found")
	}

	// Verify migration recorded
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 13").Scan(&n); err != nil || n != 1 {
		t.Fatalf("v13 not recorded: n=%d err=%v", n, err)
	}
}
