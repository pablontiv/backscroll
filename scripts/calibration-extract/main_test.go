package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

// newTestDB creates a temporary database for testing
func newTestDB(t *testing.T) (*storage.Database, func()) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return db, func() { _ = db.Close() }
}

// createTestCorrectionSignals populates a test DB with correction candidates
func createTestCorrectionSignals(t *testing.T, db *storage.Database, signals []struct {
	path       string
	ordinal    int
	detector   string
	confidence float64
	text       string
	uuid       string
}) {
	// Create session files with indexed messages
	files := make([]storage.IndexedFile, 0)
	sessionPaths := make(map[string]int) // path -> max ordinal

	for _, sig := range signals {
		if _, exists := sessionPaths[sig.path]; !exists {
			sessionPaths[sig.path] = -1
		}
		if sig.ordinal > sessionPaths[sig.path] {
			sessionPaths[sig.path] = sig.ordinal
		}
	}

	// Build IndexedFiles with messages
	for path, maxOrd := range sessionPaths {
		msgs := make([]storage.IndexedMessage, maxOrd+1)
		for i := 0; i <= maxOrd; i++ {
			msgs[i] = storage.IndexedMessage{
				Ordinal:           i,
				Role:              "user",
				Text:              "default",
				UUID:              "",
				Timestamp:         "2026-01-01T00:00:00Z",
				ContentType:       "text",
				ExtractionVersion: 1,
			}
		}

		// Populate with actual signals
		for _, sig := range signals {
			if sig.path == path {
				if sig.ordinal <= maxOrd {
					msgs[sig.ordinal].Text = sig.text
					msgs[sig.ordinal].UUID = sig.uuid
				}
			}
		}

		files = append(files, storage.IndexedFile{
			SourcePath: path,
			Source:     "session",
			Hash:       "h1",
			Project:    "test",
			Messages:   msgs,
		})
	}

	// Sync files to DB
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("sync files: %v", err)
	}
}

func TestStratifyPerDetectorEqualQuotas(t *testing.T) {
	// Test equal quota distribution: 12 lexicon + 12 denial candidates
	// --total 20 --per-detector 10 → stratify distributes ~10 per detector
	candidates := []storage.CorrectionCandidate{
		// 12 lexicon candidates across 2 sessions
		{UUID: "l1", SourcePath: "s1.jsonl", Ordinal: 0, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no, te pedí"},
		{UUID: "l2", SourcePath: "s1.jsonl", Ordinal: 1, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "te pedí"},
		{UUID: "l3", SourcePath: "s1.jsonl", Ordinal: 2, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "eso no es"},
		{UUID: "l4", SourcePath: "s1.jsonl", Ordinal: 3, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "otra vez"},
		{UUID: "l5", SourcePath: "s1.jsonl", Ordinal: 4, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "te dije"},
		{UUID: "l6", SourcePath: "s1.jsonl", Ordinal: 5, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "es mentira"},
		{UUID: "l7", SourcePath: "s2.jsonl", Ordinal: 0, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no era eso"},
		{UUID: "l8", SourcePath: "s2.jsonl", Ordinal: 1, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "de nuevo"},
		{UUID: "l9", SourcePath: "s2.jsonl", Ordinal: 2, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "not what i"},
		{UUID: "l10", SourcePath: "s2.jsonl", Ordinal: 3, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "what i asked"},
		{UUID: "l11", SourcePath: "s2.jsonl", Ordinal: 4, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "i said"},
		{UUID: "l12", SourcePath: "s2.jsonl", Ordinal: 5, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "you ignored"},

		// 12 denial candidates
		{UUID: "d1", SourcePath: "s3.jsonl", Ordinal: 0, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "denied"},
		{UUID: "d2", SourcePath: "s3.jsonl", Ordinal: 1, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "rechaza"},
		{UUID: "d3", SourcePath: "s3.jsonl", Ordinal: 2, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "access denied"},
		{UUID: "d4", SourcePath: "s4.jsonl", Ordinal: 0, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "denied"},
		{UUID: "d5", SourcePath: "s4.jsonl", Ordinal: 1, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "rechaza"},
		{UUID: "d6", SourcePath: "s4.jsonl", Ordinal: 2, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "denied access"},
		{UUID: "d7", SourcePath: "s5.jsonl", Ordinal: 0, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "denied"},
		{UUID: "d8", SourcePath: "s5.jsonl", Ordinal: 1, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "permission denied"},
		{UUID: "d9", SourcePath: "s5.jsonl", Ordinal: 2, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "no permission"},
		{UUID: "d10", SourcePath: "s6.jsonl", Ordinal: 0, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "rechaza"},
		{UUID: "d11", SourcePath: "s6.jsonl", Ordinal: 1, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "denied"},
		{UUID: "d12", SourcePath: "s6.jsonl", Ordinal: 2, Detectors: []string{"denial"}, MaxConfidence: 0.4, TextSnippet: "denied"},
	}

	// Stratify: 20 total, auto-equal (10 per detector), 10 per session
	samples := stratify(candidates, 20, 0, 10)

	if len(samples) > 20 {
		t.Errorf("want ≤20 samples, got %d", len(samples))
	}

	// Count by detector
	byDetector := make(map[string]int)
	for _, s := range samples {
		byDetector[s.Detector]++
	}

	// Each detector should have roughly equal count (within 1-2 due to session cap)
	for det, count := range byDetector {
		if count > 10 {
			t.Errorf("detector %s: got %d samples, want ≤10", det, count)
		}
	}
}

func TestStratifyPerSessionCap(t *testing.T) {
	// 3 sessions with 5 signals each → --per-session 2 → each session ≤2 rows
	candidates := []storage.CorrectionCandidate{
		// Session 0: 5 signals
		{UUID: "s0_0", SourcePath: "session0.jsonl", Ordinal: 0, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no1"},
		{UUID: "s0_1", SourcePath: "session0.jsonl", Ordinal: 1, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no2"},
		{UUID: "s0_2", SourcePath: "session0.jsonl", Ordinal: 2, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no3"},
		{UUID: "s0_3", SourcePath: "session0.jsonl", Ordinal: 3, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no4"},
		{UUID: "s0_4", SourcePath: "session0.jsonl", Ordinal: 4, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no5"},
		// Session 1: 5 signals
		{UUID: "s1_0", SourcePath: "session1.jsonl", Ordinal: 0, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no1"},
		{UUID: "s1_1", SourcePath: "session1.jsonl", Ordinal: 1, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no2"},
		{UUID: "s1_2", SourcePath: "session1.jsonl", Ordinal: 2, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no3"},
		{UUID: "s1_3", SourcePath: "session1.jsonl", Ordinal: 3, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no4"},
		{UUID: "s1_4", SourcePath: "session1.jsonl", Ordinal: 4, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no5"},
		// Session 2: 5 signals
		{UUID: "s2_0", SourcePath: "session2.jsonl", Ordinal: 0, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no1"},
		{UUID: "s2_1", SourcePath: "session2.jsonl", Ordinal: 1, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no2"},
		{UUID: "s2_2", SourcePath: "session2.jsonl", Ordinal: 2, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no3"},
		{UUID: "s2_3", SourcePath: "session2.jsonl", Ordinal: 3, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no4"},
		{UUID: "s2_4", SourcePath: "session2.jsonl", Ordinal: 4, Detectors: []string{"lexicon"}, MaxConfidence: 0.8, TextSnippet: "no5"},
	}

	// Stratify: 10 total, 10 per detector, 2 per session
	samples := stratify(candidates, 10, 10, 2)

	// Count by session
	bySession := make(map[string]int)
	for _, s := range samples {
		bySession[s.SourcePath]++
	}

	for session, count := range bySession {
		if count > 2 {
			t.Errorf("session %s: got %d samples, want ≤2", session, count)
		}
	}
}

func TestStratifyDeterministicOrdering(t *testing.T) {
	// Same DB + flags twice → identical CSV output
	db, cleanup := newTestDB(t)
	defer cleanup()

	signals := []struct {
		path       string
		ordinal    int
		detector   string
		confidence float64
		text       string
		uuid       string
	}{
		{"s1.jsonl", 0, "lexicon", 0.8, "no, te pedí", "u1"},
		{"s2.jsonl", 0, "denial", 0.4, "denied", "u2"},
		{"s1.jsonl", 1, "rephrase", 0.6, "rephrase", "u3"},
	}

	createTestCorrectionSignals(t, db, signals)

	opts := storage.CorrectionAggOpts{
		MinConfidence: 0.4,
	}
	candidates, err := db.AggregateCorrections(opts)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	// First run
	samples1 := stratify(candidates, 10, 0, 10)

	// Second run (with same data)
	samples2 := stratify(candidates, 10, 0, 10)

	// Should be identical
	if len(samples1) != len(samples2) {
		t.Errorf("different lengths: %d vs %d", len(samples1), len(samples2))
	}

	for i, s1 := range samples1 {
		if i >= len(samples2) {
			break
		}
		s2 := samples2[i]
		if s1.UUID != s2.UUID || s1.Detector != s2.Detector {
			t.Errorf("ordering differs at index %d: %v vs %v", i, s1, s2)
		}
	}
}

func TestWriteCSVFormat(t *testing.T) {
	// Verify CSV output has correct columns and format
	tmpFile := filepath.Join(t.TempDir(), "test.csv")

	samples := []Sample{
		{
			UUID:        "u1",
			Kind:        "correction",
			Label:       "correction",
			Detector:    "lexicon",
			Confidence:  0.8,
			SourcePath:  "s1.jsonl",
			Ordinal:     0,
			SessionTag:  "test",
			TextPreview: "no, te pedí",
		},
	}

	if err := writeCSV(tmpFile, samples); err != nil {
		t.Fatalf("writeCSV: %v", err)
	}

	// Read back and verify
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := string(content)
	if !contains(lines, "uuid,kind,label,detector,confidence,source_path,ordinal") {
		t.Error("CSV header incorrect")
	}

	if !contains(lines, "u1,correction,correction,lexicon,0.8,s1.jsonl,0") {
		t.Error("CSV row incorrect")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestWindowContextPopulation(t *testing.T) {
	// Table-driven: window context population filters tool rows and stubs <20 chars
	tests := []struct {
		name         string
		messages     []storage.IndexedMessage
		expectBefore string
		expectAfter  string
	}{
		{
			name: "populated windows skip tool rows and stubs",
			messages: []storage.IndexedMessage{
				{Ordinal: 0, Role: "assistant", Text: "[ by ai]", UUID: "u0", Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
				{Ordinal: 1, Role: "assistant", Text: "This is a substantive assistant message with enough text", UUID: "u1", Timestamp: "2026-01-01T00:01:00Z", ContentType: "text", ExtractionVersion: 1},
				{Ordinal: 2, Role: "user", Text: "no, te pedí otra cosa completamente diferente", UUID: "u2", Timestamp: "2026-01-01T00:02:00Z", ContentType: "text", ExtractionVersion: 1},
				{Ordinal: 3, Role: "assistant", Text: "tool output here", UUID: "u3", Timestamp: "2026-01-01T00:03:00Z", ContentType: "tool", ExtractionVersion: 1},
				{Ordinal: 4, Role: "user", Text: "ok", UUID: "u4", Timestamp: "2026-01-01T00:04:00Z", ContentType: "text", ExtractionVersion: 1},
				{Ordinal: 5, Role: "user", Text: "User response with more context and information for labeling", UUID: "u5", Timestamp: "2026-01-01T00:05:00Z", ContentType: "text", ExtractionVersion: 1},
			},
			expectBefore: "substantive assistant message",
			expectAfter:  "User response with more context",
		},
		{
			name: "no qualifying context → empty strings",
			messages: []storage.IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "no, te pedí otra cosa distinta aquí", UUID: "u0", Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1},
			},
			expectBefore: "",
			expectAfter:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, cleanup := newTestDB(t)
			defer cleanup()

			if err := db.SyncFiles([]storage.IndexedFile{{
				SourcePath: "test.jsonl",
				Source:     "session",
				Hash:       "h1",
				Project:    "test",
				Messages:   tt.messages,
			}}); err != nil {
				t.Fatalf("sync: %v", err)
			}

			candidates, err := db.AggregateCorrections(storage.CorrectionAggOpts{MinConfidence: 0.4})
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			if len(candidates) == 0 {
				t.Fatal("no candidates")
			}

			samples := stratifyWithDB(db, candidates, 10, 0, 10)
			if len(samples) == 0 {
				t.Fatal("no samples")
			}
			s := samples[0]

			if tt.expectBefore == "" {
				if s.WindowBefore != "" {
					t.Errorf("expect empty WindowBefore, got: %q", s.WindowBefore)
				}
			} else if !contains(s.WindowBefore, tt.expectBefore) {
				t.Errorf("expect WindowBefore contains %q, got: %q", tt.expectBefore, s.WindowBefore)
			}

			if tt.expectAfter == "" {
				if s.WindowAfter != "" {
					t.Errorf("expect empty WindowAfter, got: %q", s.WindowAfter)
				}
			} else if !contains(s.WindowAfter, tt.expectAfter) {
				t.Errorf("expect WindowAfter contains %q, got: %q", tt.expectAfter, s.WindowAfter)
			}
		})
	}
}
