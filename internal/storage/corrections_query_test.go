package storage

import (
	"path/filepath"
	"testing"
)

func TestAggregateCorrections(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync a session with multiple corrections
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
			{Ordinal: 2, Role: "user", Text: "no, te pedí otra cosa", UUID: "u2",
				Timestamp: "2026-01-01T00:00:02Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 3, Role: "assistant", Text: "Understood", UUID: "u3",
				Timestamp: "2026-01-01T00:00:03Z", ContentType: "text", ExtractionVersion: 1},
			{Ordinal: 4, Role: "user", Text: "no, te pedí", UUID: "u4", WasInterrupted: true,
				Timestamp: "2026-01-01T00:00:04Z", ContentType: "text", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Query corrections
	candidates, err := db.AggregateCorrections(CorrectionAggOpts{
		Project:       "proj",
		MinConfidence: 0.0,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(candidates) != 2 {
		t.Fatalf("expected 2 corrections (ordinal 2 + 4), got %d", len(candidates))
	}

	// Both ordinals 2 and 4 should be present with max confidence 0.8
	ordinals := []int{candidates[0].Ordinal, candidates[1].Ordinal}
	expectedOrdinals := []int{2, 4}

	// Check that both ordinals are present (order may vary)
	found := 0
	for _, expected := range expectedOrdinals {
		for _, ord := range ordinals {
			if ord == expected {
				found++
				break
			}
		}
	}
	if found != 2 {
		t.Errorf("expected ordinals 2 and 4, got %v", ordinals)
	}

	// Check that at least one candidate has detectors populated
	hasDetectors := false
	for _, c := range candidates {
		if len(c.Detectors) > 0 {
			hasDetectors = true
			break
		}
	}
	if !hasDetectors {
		t.Error("expected at least one candidate with detectors slice populated")
	}
}
