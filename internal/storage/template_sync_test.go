package storage

import (
	"path/filepath"
	"testing"
)

func TestSyncFilesMinesTemplates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Two Bash tool outputs with same error pattern (different numbers).
	// After mining, should collapse into one template.
	msg1 := IndexedMessage{Ordinal: 0, Role: "user", Text: "run bash", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text",
		ToolName: "", IsError: nil, ExtractionVersion: 1}
	msg2 := IndexedMessage{Ordinal: 1, Role: "assistant", Text: "error: connection refused 127.0.0.1:8080",
		UUID: "u2", Timestamp: "2026-01-01T00:00:01Z", ContentType: "tool",
		ToolName: "Bash", IsError: boolPtr(true), ExtractionVersion: 1}
	msg3 := IndexedMessage{Ordinal: 2, Role: "assistant", Text: "error: connection refused 127.0.0.1:9000",
		UUID: "u3", Timestamp: "2026-01-01T00:00:02Z", ContentType: "tool",
		ToolName: "Bash", IsError: boolPtr(true), ExtractionVersion: 1}

	files := []IndexedFile{{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{msg1, msg2, msg3}}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Query: should have exactly 1 template (the two error lines collapsed).
	var tmplCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&tmplCount); err != nil {
		t.Fatal(err)
	}
	if tmplCount != 1 {
		t.Errorf("want 1 template, got %d", tmplCount)
	}

	// Matches should point to both u2 and u3 (occurrence_count derived from matches at query time).
	var matchCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matchCount); err != nil {
		t.Fatal(err)
	}
	if matchCount != 2 {
		t.Errorf("want 2 template matches, got %d", matchCount)
	}
}

func TestSyncFilesIdempotency(t *testing.T) {
	// Re-syncing the same file twice should not inflate template counts.
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msg := IndexedMessage{Ordinal: 0, Role: "assistant", Text: "error: timeout after 30 seconds",
		UUID: "u1", Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
		ToolName: "Bash", IsError: boolPtr(true), ExtractionVersion: 1}

	files := []IndexedFile{{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{msg}}}

	// Sync once
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	var matchesBefore int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matchesBefore); err != nil {
		t.Fatal(err)
	}

	// Sync again (same file, same hash)
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	var matchesAfter int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matchesAfter); err != nil {
		t.Fatal(err)
	}

	// Matches should NOT have doubled (UNIQUE on template_matches ensures idempotency).
	if matchesAfter != matchesBefore {
		t.Errorf("re-sync inflated matches: %d -> %d", matchesBefore, matchesAfter)
	}
}

func TestSyncFilesWithNonErrorMessages(t *testing.T) {
	// Non-error tool messages should not be mined.
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msgs := []IndexedMessage{
		{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(false),
			Text: "successful execution", ExtractionVersion: 1},
		{Ordinal: 1, UUID: "u2", ToolName: "Bash", IsError: nil,
			Text: "another message", ExtractionVersion: 1},
	}
	files := []IndexedFile{{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: msgs}}

	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// No templates should be created
	var tmplCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM message_templates`).Scan(&tmplCount); err != nil {
		t.Fatal(err)
	}
	if tmplCount != 0 {
		t.Errorf("want 0 templates for non-error messages, got %d", tmplCount)
	}
}

func TestSyncFilesWipeReloadOrphans(t *testing.T) {
	// Wipe-reload should delete orphaned template_matches for purged rows
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msg := IndexedMessage{Ordinal: 0, UUID: "", ToolName: "Bash", IsError: boolPtr(true),
		Text: "error: failed", ExtractionVersion: 1}

	files := []IndexedFile{{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{msg}}}

	// First sync: legacy (no UUID)
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Resync with UUID (wipe-reload branch should delete orphaned matches)
	msg.UUID = "u1"
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("after wipe-reload: expect 1 match, got %d", count)
	}
}

func TestSyncFilesTemplateUpdatePath(t *testing.T) {
	// Sync two files with same template in different files; count should increase.
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msg := IndexedMessage{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true),
		Text: "error: database locked", ExtractionVersion: 1}

	// First file
	files1 := []IndexedFile{{SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{msg}}}
	if err := db.SyncFiles(files1); err != nil {
		t.Fatal(err)
	}

	// Second file with same template
	msg.UUID = "u2"
	files2 := []IndexedFile{{SourcePath: "/p/s2.jsonl", Source: "session", Hash: "h2", Project: "proj",
		Messages: []IndexedMessage{msg}}}
	if err := db.SyncFiles(files2); err != nil {
		t.Fatal(err)
	}

	// Should have 1 template with 2 matches (count derived at query time)
	var matchCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matchCount); err != nil {
		t.Fatal(err)
	}
	if matchCount != 2 {
		t.Errorf("match count = %d, want 2", matchCount)
	}
}

func TestSyncFilesUNIQUEConflict(t *testing.T) {
	// Test that UNIQUE conflict on template_matches is handled gracefully
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	msg := IndexedMessage{Ordinal: 0, UUID: "u1", ToolName: "Bash", IsError: boolPtr(true),
		Text: "error: locked", ExtractionVersion: 1}

	files := []IndexedFile{{SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{msg}}}

	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Sync identical message again - should not error
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var matchCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM template_matches`).Scan(&matchCount); err != nil {
		t.Fatal(err)
	}
	if matchCount != 1 {
		t.Errorf("expect 1 match after duplicate sync, got %d", matchCount)
	}
}
