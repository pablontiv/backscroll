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

// TestAggregateCorrectionNullUUID tests that AggregateCorrections handles
// search_items rows with NULL uuid (legacy/B3-backfilled rows).
// This reproduces the bug: "scan correction: sql: Scan error on column index 5, name "uuid": converting NULL to string is unsupported".
func TestAggregateCorrectionNullUUID(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert a search_items row with NULL uuid (legacy or B3-backfilled row)
	_, err = db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, uuid, project, content_type)
		VALUES ('session', '/p/legacy.jsonl', 0, 'user', 'Corrected message', '2026-01-01T00:00:00Z', NULL, 'proj', 'text')
	`)
	if err != nil {
		t.Fatalf("insert search_items row: %v", err)
	}

	// Insert a UUID-less correction_signals row for that legacy message.
	_, err = db.db.Exec(`
		INSERT INTO correction_signals (item_uuid, source_path, ordinal, detector, confidence, extraction_version)
		VALUES ('', '/p/legacy.jsonl', 0, 'lexicon', 0.8, 1)
	`)
	if err != nil {
		t.Fatalf("insert correction_signals row: %v", err)
	}

	// Query corrections with MinConfidence 0 to include all candidates
	candidates, err := db.AggregateCorrections(CorrectionAggOpts{
		Project:       "proj",
		MinConfidence: 0.0,
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("expected 1 correction, got %d", len(candidates))
	}

	c := candidates[0]
	if c.Ordinal != 0 {
		t.Errorf("expected ordinal 0, got %d", c.Ordinal)
	}
	// NULL uuid should be returned as empty string (via COALESCE)
	if c.UUID != "" {
		t.Errorf("expected empty UUID for NULL row, got %q", c.UUID)
	}
	if len(c.Detectors) != 1 || c.Detectors[0] != "lexicon" {
		t.Errorf("expected detectors=[\"lexicon\"], got %v", c.Detectors)
	}
	if c.MaxConfidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %v", c.MaxConfidence)
	}

	if err := db.UpsertAnnotation("", "/p/legacy.jsonl", 0, "correction", "reviewed"); err != nil {
		t.Fatalf("annotate legacy correction: %v", err)
	}
	pending, err := db.AggregateCorrections(CorrectionAggOpts{PendingOnly: true})
	if err != nil {
		t.Fatalf("aggregate pending legacy corrections: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending legacy corrections = %+v, want none", pending)
	}
}

func TestAggregateCorrectionsUUIDLessSignalFallbacks(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, row := range []struct {
		path       string
		uuid       string
		signalUUID any
	}{
		{path: "/p/fallback-null.jsonl", uuid: "null-signal-target", signalUUID: nil},
		{path: "/p/fallback-empty.jsonl", uuid: "empty-signal-target", signalUUID: ""},
	} {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
				(source, source_path, ordinal, role, text, uuid, project, content_type)
			VALUES ('session', ?, 3, 'user', ?, ?, 'proj', 'text')
		`, row.path, row.uuid+" text", row.uuid); err != nil {
			t.Fatalf("insert search item %q: %v", row.uuid, err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals
				(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES (?, ?, 3, 'test', 0.8, 1)
		`, row.signalUUID, row.path); err != nil {
			t.Fatalf("insert signal for %q: %v", row.uuid, err)
		}
	}

	got, err := db.AggregateCorrections(CorrectionAggOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("UUID-less fallback corrections = %+v, want two", got)
	}
	seen := map[string]bool{}
	for _, candidate := range got {
		seen[candidate.UUID] = true
	}
	for _, uuid := range []string{"null-signal-target", "empty-signal-target"} {
		if !seen[uuid] {
			t.Errorf("UUID-less fallback omitted %q: %+v", uuid, got)
		}
	}
}

func TestAggregateCorrectionsUUIDLessSignalOmitsAmbiguousCoordinates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	for _, row := range []struct {
		path       string
		signalUUID any
		uuids      []any
	}{
		{path: "/p/ambiguous-null.jsonl", signalUUID: nil, uuids: []any{nil, "current-null"}},
		{path: "/p/ambiguous-empty.jsonl", signalUUID: "", uuids: []any{"", "current-empty"}},
	} {
		for i, uuid := range row.uuids {
			if _, err := db.db.Exec(`
				INSERT INTO search_items
					(source, source_path, ordinal, role, text, uuid, project, content_type)
				VALUES ('session', ?, 7, 'user', ?, ?, 'proj', 'text')
			`, row.path, row.path+string(rune('a'+i)), uuid); err != nil {
				t.Fatalf("insert ambiguous search item at %q: %v", row.path, err)
			}
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals
				(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES (?, ?, 7, 'test', 0.8, 1)
		`, row.signalUUID, row.path); err != nil {
			t.Fatalf("insert ambiguous signal at %q: %v", row.path, err)
		}
	}

	got, err := db.AggregateCorrections(CorrectionAggOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("ambiguous UUID-less corrections = %+v, want none", got)
	}
}

func TestAggregateCorrectionsStableTiePagination(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	rows := []struct {
		path     string
		ordinal  int
		uuid     string
		detector string
	}{
		{path: "/b.jsonl", ordinal: 0, uuid: "x", detector: "d-x"},
		{path: "/a.jsonl", ordinal: 1, uuid: "z", detector: "d-z"},
		{path: "/a.jsonl", ordinal: 0, uuid: "b", detector: "d-b"},
		{path: "/a.jsonl", ordinal: 0, uuid: "a", detector: "d-a"},
	}
	for _, row := range rows {
		if _, err := db.db.Exec(`
			INSERT INTO search_items
				(source, source_path, ordinal, role, text, uuid, project, content_type)
			VALUES ('session', ?, ?, 'user', ?, ?, 'proj', 'text')
		`, row.path, row.ordinal, row.uuid+" text", row.uuid); err != nil {
			t.Fatalf("insert search item %q: %v", row.uuid, err)
		}
		if _, err := db.db.Exec(`
			INSERT INTO correction_signals
				(item_uuid, source_path, ordinal, detector, confidence, extraction_version)
			VALUES (?, ?, ?, ?, 0.8, 1)
		`, row.uuid, row.path, row.ordinal, row.detector); err != nil {
			t.Fatalf("insert signal %q: %v", row.uuid, err)
		}
	}

	want := []string{"a", "b", "z", "x"}
	for pass := 0; pass < 2; pass++ {
		for offset, wantUUID := range want {
			got, err := db.AggregateCorrections(CorrectionAggOpts{Limit: 1, Offset: offset})
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 || got[0].UUID != wantUUID {
				t.Fatalf("pass %d page %d = %+v, want UUID %q", pass, offset, got, wantUUID)
			}
		}
	}
}
