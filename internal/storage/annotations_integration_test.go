package storage

import (
	"path/filepath"
	"testing"
)

func TestAnnotationLoopSimulation(t *testing.T) {
	// Simulates: sync -> query pending -> annotate each -> query again -> all annotated
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync a session with 3 messages
	files := []IndexedFile{{
		SourcePath: "/p/session.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "myproj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "Do X", UUID: "u0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "Done", UUID: "u1",
				Timestamp: "2026-01-01T00:00:01Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 2, Role: "user", Text: "no, te pedí otra cosa", UUID: "u2",
				Timestamp: "2026-01-01T00:00:02Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 3, Role: "assistant", Text: "Trying again", UUID: "u3",
				Timestamp: "2026-01-01T00:00:03Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 4, Role: "user", Text: "no, te pedí", UUID: "u4",
				Timestamp: "2026-01-01T00:00:04Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Insert two correction signals for ordinals 2 and 4
	if _, err := db.db.Exec(`INSERT INTO correction_signals
		(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('u2', '/p/session.jsonl', 2, 'detector1', 0.8, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`INSERT INTO correction_signals
		(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('u4', '/p/session.jsonl', 4, 'detector1', 0.7, 1)`); err != nil {
		t.Fatal(err)
	}

	// Initial query: all pending
	pending, err := db.AggregateCorrections(CorrectionAggOpts{
		Project:       "myproj",
		MinConfidence: 0.0,
		Limit:         10,
		PendingOnly:   true,
	})
	if err != nil {
		t.Fatalf("initial aggregate: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}

	// Agent loop: annotate first batch
	for i, cand := range pending {
		label := "needs_clarification"
		if i > 0 {
			label = "agent_misunderstood"
		}
		if err := db.UpsertAnnotation(cand.UUID, cand.SourcePath, cand.Ordinal, "correction", label); err != nil {
			t.Fatalf("annotate %d: %v", i, err)
		}
	}

	// Query again: should be empty
	pending, err = db.AggregateCorrections(CorrectionAggOpts{
		Project:       "myproj",
		MinConfidence: 0.0,
		Limit:         10,
		PendingOnly:   true,
	})
	if err != nil {
		t.Fatalf("second aggregate: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending after annotation, got %d", len(pending))
	}
}

func TestAnnotationIdempotency(t *testing.T) {
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
			{Ordinal: 0, Role: "user", Text: "no, te pedí", UUID: "u1",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Annotate twice with different labels
	if err := db.UpsertAnnotation("u1", "/p/s.jsonl", 0, "correction", "first_label"); err != nil {
		t.Fatal(err)
	}

	if err := db.UpsertAnnotation("u1", "/p/s.jsonl", 0, "correction", "second_label"); err != nil {
		t.Fatal(err)
	}

	// Should have exactly one row with the latest label
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM annotations WHERE source_path = '/p/s.jsonl' AND ordinal = 0 AND kind = 'correction'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 annotation row (replacement), got %d", n)
	}

	var label string
	if err := db.db.QueryRow("SELECT label FROM annotations WHERE source_path = '/p/s.jsonl' AND ordinal = 0 AND kind = 'correction'").Scan(&label); err != nil {
		t.Fatal(err)
	}
	if label != "second_label" {
		t.Errorf("expected latest label 'second_label', got %q", label)
	}
}
