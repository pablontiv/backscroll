package storage

import (
	"path/filepath"
	"testing"
)

func TestUpsertAnnotation(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync a session with a correction candidate
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

	// Upsert an annotation
	if err := db.UpsertAnnotation("u1", "/p/s.jsonl", 0, "correction", "fixable_issue"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Verify it was written
	var label string
	if err := db.db.QueryRow("SELECT label FROM annotations WHERE item_uuid = 'u1' AND kind = 'correction'").Scan(&label); err != nil {
		t.Fatalf("query: %v", err)
	}
	if label != "fixable_issue" {
		t.Errorf("expected label 'fixable_issue', got %q", label)
	}

	// Upsert same message, kind, new label (replace)
	if err := db.UpsertAnnotation("u1", "/p/s.jsonl", 0, "correction", "not_an_issue"); err != nil {
		t.Fatalf("replace: %v", err)
	}

	if err := db.db.QueryRow("SELECT label FROM annotations WHERE item_uuid = 'u1' AND kind = 'correction'").Scan(&label); err != nil {
		t.Fatalf("query after replace: %v", err)
	}
	if label != "not_an_issue" {
		t.Errorf("replace failed: expected 'not_an_issue', got %q", label)
	}
}

func TestUpsertAnnotationValidatesMessageExists(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Try to annotate a non-existent message
	err = db.UpsertAnnotation("nonexistent_uuid", "/p/missing.jsonl", 0, "correction", "label")
	if err == nil {
		t.Fatal("expected error for missing message")
	}
}

func TestAggregateCorrectionsWithPendingFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync a session with two messages that will be correction candidates
	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "no, te pedí", UUID: "u1",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 1, Role: "user", Text: "no, te pedí otra cosa", UUID: "u2",
				Timestamp: "2026-01-01T00:00:01Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Insert two correction signals
	if _, err := db.db.Exec(`INSERT INTO correction_signals
		(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('u1', '/p/s.jsonl', 0, 'detector1', 0.8, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`INSERT INTO correction_signals
		(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('u2', '/p/s.jsonl', 1, 'detector1', 0.7, 1)`); err != nil {
		t.Fatal(err)
	}

	// Both candidates exist initially
	all, err := db.AggregateCorrections(CorrectionAggOpts{
		Project:       "proj",
		MinConfidence: 0.0,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("aggregate all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 corrections, got %d", len(all))
	}

	// Annotate the first one
	if err := db.UpsertAnnotation("u1", "/p/s.jsonl", 0, "correction", "reviewed"); err != nil {
		t.Fatal(err)
	}

	// Query pending: only u2 should remain
	pending, err := db.AggregateCorrections(CorrectionAggOpts{
		Project:       "proj",
		MinConfidence: 0.0,
		Limit:         10,
		PendingOnly:   true,
	})
	if err != nil {
		t.Fatalf("aggregate pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].UUID != "u2" {
		t.Errorf("expected pending uuid u2, got %q", pending[0].UUID)
	}
}

func TestUpsertAnnotationResolvesByUUID(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync a session
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

	// Annotate by uuid only (no path/ordinal provided by caller)
	if err := db.UpsertAnnotation("u1", "", -1, "correction", "fixable"); err != nil {
		t.Fatalf("upsert by uuid: %v", err)
	}

	// Verify the annotation was written with the RESOLVED (source_path, ordinal)
	var label string
	if err := db.db.QueryRow("SELECT label FROM annotations WHERE source_path = '/p/s.jsonl' AND ordinal = 0 AND kind = 'correction'").Scan(&label); err != nil {
		t.Fatalf("query by resolved coords: %v", err)
	}
	if label != "fixable" {
		t.Errorf("expected label 'fixable', got %q", label)
	}

	// Verify the annotation is indexed by uuid
	if err := db.db.QueryRow("SELECT label FROM annotations WHERE item_uuid = 'u1' AND kind = 'correction'").Scan(&label); err != nil {
		t.Fatalf("query by uuid: %v", err)
	}
	if label != "fixable" {
		t.Errorf("expected label 'fixable', got %q", label)
	}
}

func TestUpsertAnnotationConflictingUUIDAndCoords(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync two messages
	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "msg1", UUID: "u1",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 1, Role: "user", Text: "msg2", UUID: "u2",
				Timestamp: "2026-01-01T00:00:01Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Try to annotate with uuid=u1 but path/ordinal pointing to u2
	err = db.UpsertAnnotation("u1", "/p/s.jsonl", 1, "correction", "label")
	if err == nil {
		t.Fatal("expected error for conflicting uuid and coords")
	}
}

func TestPurgeDeletesAnnotationsExplicitly(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Old message with annotation
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

	// Annotate it
	if err := db.UpsertAnnotation("u1", "/p/old.jsonl", 0, "correction", "reviewed"); err != nil {
		t.Fatal(err)
	}

	// Purge old entries
	if _, err := db.Purge("2021-01-01"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	// Annotations should be gone
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM annotations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("purge must delete annotations, %d remain", n)
	}
}
