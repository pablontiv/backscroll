package storage

import (
	"path/filepath"
	"testing"
)

func TestV11MigrationAddsCorrectionSignals(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// correction_signals exists with expected columns
	if _, err := db.db.Exec(`INSERT INTO correction_signals
		(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('u1', '/p/s.jsonl', 0, 'lexicon', 0.8, 1)`); err != nil {
		t.Fatalf("insert correction_signals: %v", err)
	}

	// UNIQUE(source_path, ordinal, detector) enforced
	if _, err := db.db.Exec(`INSERT INTO correction_signals
		(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('u2', '/p/s.jsonl', 0, 'lexicon', 0.7, 1)`); err == nil {
		t.Fatal("expected UNIQUE(source_path, ordinal, detector) violation")
	}

	// index on detector exists
	if _, err := db.db.Exec(`SELECT 1 FROM correction_signals WHERE detector = 'lexicon'`); err != nil {
		t.Fatalf("detector index missing: %v", err)
	}

	// migration recorded
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 11").Scan(&n); err != nil || n != 1 {
		t.Fatalf("v11 not recorded: n=%d err=%v", n, err)
	}
}
