package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/sequences"
)

func TestLoadToolSequences(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync a session with tool events
	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl",
		Source:     "session",
		Hash:       "h1",
		Project:    "proj",
		Messages: []IndexedMessage{
			{
				Ordinal:           0,
				Role:              "user",
				Text:              "run tests",
				UUID:              "u0",
				Timestamp:         "2026-01-01T00:00:00Z",
				ContentType:       "text",
				ExtractionVersion: 1,
			},
			{
				Ordinal:           1,
				Role:              "assistant",
				Text:              "Bash command=go test",
				UUID:              "u1",
				Timestamp:         "2026-01-01T00:00:01Z",
				ContentType:       "tool",
				ToolName:          "Bash",
				CommandHead:       "test",
				ExtractionVersion: 1,
			},
			{
				Ordinal:           2,
				Role:              "assistant",
				Text:              "Read /path",
				UUID:              "u2",
				Timestamp:         "2026-01-01T00:00:02Z",
				ContentType:       "tool",
				ToolName:          "Read",
				CommandHead:       "",
				ExtractionVersion: 1,
			},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	seqs, err := db.LoadToolSequences(LoadSequencesOpts{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(seqs) != 1 {
		t.Fatalf("want 1 session, got %d", len(seqs))
	}

	seq := seqs[0]
	if seq.SessionID != "/p/s.jsonl" {
		t.Errorf("session id: %q", seq.SessionID)
	}

	// Sequence should be: [GO_EXEC, FILE_READ] (categorized)
	if len(seq.Items) != 2 {
		t.Fatalf("want 2 items, got %d: %v", len(seq.Items), seq.Items)
	}

	// Check actual categories from default mapping
	if seq.Items[0] != "GO_EXEC" {
		t.Errorf("first item: %q, want GO_EXEC", seq.Items[0])
	}
	if seq.Items[1] != "FILE_READ" {
		t.Errorf("second item: %q, want FILE_READ", seq.Items[1])
	}
}

func TestLoadToolSequencesMultipleSessions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Two sessions with different tool sequences
	files := []IndexedFile{
		{
			SourcePath: "/p/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj",
			Messages: []IndexedMessage{
				{
					Ordinal:           0,
					Role:              "assistant",
					Text:              "Bash cmd",
					UUID:              "u0",
					Timestamp:         "2026-01-01T00:00:00Z",
					ContentType:       "tool",
					ToolName:          "Read",
					CommandHead:       "",
					ExtractionVersion: 1,
				},
				{
					Ordinal:           1,
					Role:              "assistant",
					Text:              "Write file",
					UUID:              "u1",
					Timestamp:         "2026-01-01T00:00:01Z",
					ContentType:       "tool",
					ToolName:          "Write",
					CommandHead:       "",
					ExtractionVersion: 1,
				},
			},
		},
		{
			SourcePath: "/p/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "proj",
			Messages: []IndexedMessage{
				{
					Ordinal:           0,
					Role:              "assistant",
					Text:              "Read again",
					UUID:              "u2",
					Timestamp:         "2026-01-01T00:00:02Z",
					ContentType:       "tool",
					ToolName:          "Read",
					CommandHead:       "",
					ExtractionVersion: 1,
				},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	seqs, err := db.LoadToolSequences(LoadSequencesOpts{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(seqs) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(seqs))
	}

	// All should map to FILE_READ or FILE_WRITE categories
	for _, seq := range seqs {
		for _, item := range seq.Items {
			if item != "FILE_READ" && item != "FILE_WRITE" {
				t.Errorf("unexpected category: %q", item)
			}
		}
	}
}

func TestLoadToolSequencesMiningIntegration(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Create sequences where Read→Write pattern appears in 2 sessions
	files := []IndexedFile{
		{
			SourcePath: "/p/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", UUID: "u0", Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool", ToolName: "Read", ExtractionVersion: 1},
				{Ordinal: 1, Role: "assistant", UUID: "u1", Timestamp: "2026-01-01T00:00:01Z", ContentType: "tool", ToolName: "Write", ExtractionVersion: 1},
			},
		},
		{
			SourcePath: "/p/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "proj",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", UUID: "u2", Timestamp: "2026-01-01T00:00:02Z", ContentType: "tool", ToolName: "Read", ExtractionVersion: 1},
				{Ordinal: 1, Role: "assistant", UUID: "u3", Timestamp: "2026-01-01T00:00:03Z", ContentType: "tool", ToolName: "Write", ExtractionVersion: 1},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	seqs, err := db.LoadToolSequences(LoadSequencesOpts{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Mine for frequent sequences
	patterns := sequences.Mine(seqs, 2, 2, 6)
	if len(patterns) == 0 {
		t.Error("expected patterns")
	}

	// Should find READ→WRITE with support 2
	found := false
	for _, p := range patterns {
		if len(p.Items) == 2 && p.Items[0] == "FILE_READ" && p.Items[1] == "FILE_WRITE" && p.Support == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected FILE_READ→FILE_WRITE pattern, got: %+v", patterns)
	}
}

func TestLoadToolSequencesWithProjectFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Two sessions in different projects
	files := []IndexedFile{
		{
			SourcePath: "/p1/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", UUID: "u0", Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool", ToolName: "Read", ExtractionVersion: 1},
			},
		},
		{
			SourcePath: "/p2/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", UUID: "u1", Timestamp: "2026-01-01T00:00:01Z", ContentType: "tool", ToolName: "Write", ExtractionVersion: 1},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Load all
	seqs, err := db.LoadToolSequences(LoadSequencesOpts{})
	if err != nil {
		t.Fatalf("load all: %v", err)
	}
	if len(seqs) != 2 {
		t.Errorf("want 2 sessions, got %d", len(seqs))
	}

	// Load proj1 only
	seqs, err = db.LoadToolSequences(LoadSequencesOpts{Project: "proj1"})
	if err != nil {
		t.Fatalf("load proj1: %v", err)
	}
	if len(seqs) != 1 {
		t.Errorf("want 1 session in proj1, got %d", len(seqs))
	}
	if seqs[0].SessionID != "/p1/s1.jsonl" {
		t.Errorf("wrong session: %q", seqs[0].SessionID)
	}
}

func TestLoadToolSequencesWithDateFilters(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{
		{
			SourcePath: "/p/s1.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", UUID: "u0", Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool", ToolName: "Read", ExtractionVersion: 1},
			},
		},
		{
			SourcePath: "/p/s2.jsonl",
			Source:     "session",
			Hash:       "h2",
			Project:    "proj",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", UUID: "u1", Timestamp: "2026-01-10T00:00:00Z", ContentType: "tool", ToolName: "Write", ExtractionVersion: 1},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Load after 2026-01-05
	seqs, err := db.LoadToolSequences(LoadSequencesOpts{After: "2026-01-05T00:00:00Z"})
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if len(seqs) != 1 {
		t.Errorf("want 1 session after 2026-01-05, got %d", len(seqs))
	}

	// Load before 2026-01-05
	seqs, err = db.LoadToolSequences(LoadSequencesOpts{Before: "2026-01-05T00:00:00Z"})
	if err != nil {
		t.Fatalf("load before: %v", err)
	}
	if len(seqs) != 1 {
		t.Errorf("want 1 session before 2026-01-05, got %d", len(seqs))
	}
}

func TestUpsertAnnotationEmptyBothUuidAndPath(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Try to upsert with both uuid and path empty
	err = db.UpsertAnnotation("", "", 0, "correction", "test-label")
	if err == nil {
		t.Error("expected error for empty uuid and path, got nil")
	}
	t.Logf("error for empty uuid+path: %v", err)
}

func TestUpsertAnnotationValidPathOrdinal(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert a message first
	files := []IndexedFile{
		{
			SourcePath: "/p/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj",
			Messages: []IndexedMessage{
				{
					Ordinal:           0,
					Role:              "assistant",
					UUID:              "msg-uuid-1",
					Timestamp:         "2026-01-01T00:00:00Z",
					ContentType:       "text",
					Text:              "test",
					ExtractionVersion: 1,
				},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Now upsert annotation with path and ordinal
	err = db.UpsertAnnotation("", "/p/s.jsonl", 0, "correction", "test-label")
	if err != nil {
		t.Errorf("upsert with path+ordinal failed: %v", err)
	}
}

func TestUpsertAnnotationWithUuid(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Insert a message first
	files := []IndexedFile{
		{
			SourcePath: "/p/s.jsonl",
			Source:     "session",
			Hash:       "h1",
			Project:    "proj",
			Messages: []IndexedMessage{
				{
					Ordinal:           0,
					Role:              "assistant",
					UUID:              "msg-uuid-1",
					Timestamp:         "2026-01-01T00:00:00Z",
					ContentType:       "text",
					Text:              "test",
					ExtractionVersion: 1,
				},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Upsert annotation with direct uuid
	err = db.UpsertAnnotation("msg-uuid-1", "", 0, "correction", "test-label")
	if err != nil {
		t.Errorf("upsert with uuid failed: %v", err)
	}
}

// TestLoadToolSequencesIgnoresLimitOffset pins the contract: Limit/Offset
// paginate mined patterns at the CLI layer — the input corpus must never be
// truncated (a pre-mining cut corrupts support counts nondeterministically).
func TestLoadToolSequencesIgnoresLimitOffset(t *testing.T) {
	db, err := Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	var files []IndexedFile
	for i := 0; i < 5; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/p/s%d.jsonl", i), Source: "session", Hash: fmt.Sprintf("h%d", i), Project: "proj",
			Messages: []IndexedMessage{
				{Ordinal: 0, UUID: fmt.Sprintf("sq%d-a", i), Role: "assistant", Text: "Read file_path=/x", ContentType: "tool",
					ToolName: "Read", Timestamp: "2026-01-01T00:00:00Z", ExtractionVersion: 1},
				{Ordinal: 1, UUID: fmt.Sprintf("sq%d-b", i), Role: "assistant", Text: "Edit file_path=/x", ContentType: "tool",
					ToolName: "Edit", Timestamp: "2026-01-01T00:00:01Z", ExtractionVersion: 1},
			},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	seqs, err := db.LoadToolSequences(LoadSequencesOpts{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 5 {
		t.Errorf("input corpus truncated: got %d sequences, want all 5 (Limit/Offset must not apply pre-mining)", len(seqs))
	}
}
