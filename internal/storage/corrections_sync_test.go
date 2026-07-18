package storage

import (
	"path/filepath"
	"testing"
)

func TestSyncFilesWritesCorrectionSignals(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Simulate a session with a correction: user requests something,
	// assistant does it, user says "no, te pedí otra cosa"
	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "Do X", UUID: "u0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "Doing X",
				UUID: "u1", Timestamp: "2026-01-01T00:00:01Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 2, Role: "user", Text: "no, te pedí otra cosa", UUID: "u2",
				Timestamp: "2026-01-01T00:00:02Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}

	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Verify correction_signals row was written for message 2 (lexicon detector)
	var detectorName string
	var confidence float64
	if err := db.db.QueryRow(`
		SELECT detector, confidence FROM correction_signals
		WHERE source_path = '/p/s.jsonl' AND ordinal = 2
	`).Scan(&detectorName, &confidence); err != nil {
		t.Fatalf("query correction_signals: %v", err)
	}
	if detectorName != "lexicon" || confidence != 0.8 {
		t.Errorf("expected lexicon/0.8, got %s/%f", detectorName, confidence)
	}
}

func TestPurgeDeletesCorrectionSignalsExplicitly(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Old message with correction signal
	files := []IndexedFile{{
		SourcePath: "/p/old.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "no, te pedí", UUID: "u1",
				Timestamp: "2020-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Purge("2021-01-01"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("purge must delete satellite correction_signals rows, %d remain", n)
	}
}
