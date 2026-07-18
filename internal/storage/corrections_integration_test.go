package storage

import (
	"path/filepath"
	"testing"
)

func TestCorrectionsEndToEnd(t *testing.T) {
	// Full cycle: sync -> detectors fire -> aggregate -> purge

	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/project/session.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "myproj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "Do X", UUID: "u0",
				Timestamp: "2026-01-01T10:00:00Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "Doing X", UUID: "u1",
				Timestamp: "2026-01-01T10:00:01Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 2, Role: "user", Text: "no, te pedí otra cosa", UUID: "u2",
				Timestamp: "2026-01-01T10:00:02Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}

	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Detectors should have fired
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals").Scan(&n); err != nil {
		t.Fatalf("query corrections: %v", err)
	}
	if n == 0 {
		t.Fatal("expected correction_signals to be populated after sync")
	}

	// Aggregate and verify
	candidates, err := db.AggregateCorrections(CorrectionAggOpts{
		Project:       "myproj",
		MinConfidence: 0.0,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected candidates from aggregation")
	}

	t.Logf("Aggregated %d candidates", len(candidates))

	// Purge old entries
	if _, err := db.Purge("2026-01-02"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	// Verify corrections are gone
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals").Scan(&n); err != nil {
		t.Fatalf("query after purge: %v", err)
	}
	if n != 0 {
		t.Errorf("expected corrections purged, %d remain", n)
	}
}

func TestCorrectionsIdempotent(t *testing.T) {
	// Re-sync the same session: correction_signals must not duplicate

	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "Do X", UUID: "u0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "Doing X", UUID: "u1",
				Timestamp: "2026-01-01T00:00:01Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 2, Role: "user", Text: "no, te pedí", UUID: "u2",
				Timestamp: "2026-01-01T00:00:02Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}

	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var nBefore int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals").Scan(&nBefore); err != nil {
		t.Fatal(err)
	}

	// Re-sync with same hash
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var nAfter int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM correction_signals").Scan(&nAfter); err != nil {
		t.Fatal(err)
	}

	if nBefore != nAfter {
		t.Errorf("idempotency violated: %d before, %d after re-sync", nBefore, nAfter)
	}
}
