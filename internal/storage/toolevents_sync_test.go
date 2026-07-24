package storage

import (
	"path/filepath"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestSyncFilesWritesToolEvents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "no, otra cosa", UUID: "u1", Timestamp: "2026-01-01T00:00:00Z",
				ContentType: "text", WasInterrupted: true, ExtractionVersion: CurrentExtractionVersion},
			{Ordinal: 1, Role: "assistant", Text: "Bash command=go test", UUID: "u2#t0", Timestamp: "2026-01-01T00:00:01Z",
				ContentType: "tool", ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true),
				ExtractionVersion: CurrentExtractionVersion},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var wasInterrupted, extractionVersion int
	if err := db.db.QueryRow(`SELECT was_interrupted, extraction_version FROM search_items WHERE uuid = 'u1'`).
		Scan(&wasInterrupted, &extractionVersion); err != nil {
		t.Fatalf("query u1: %v", err)
	}
	if wasInterrupted != 1 || extractionVersion != CurrentExtractionVersion {
		t.Errorf("u1: was_interrupted=%d extraction_version=%d", wasInterrupted, extractionVersion)
	}

	var toolName, commandHead string
	var isError int
	if err := db.db.QueryRow(`SELECT tool_name, command_head, is_error FROM tool_events WHERE message_uuid = 'u2#t0'`).
		Scan(&toolName, &commandHead, &isError); err != nil {
		t.Fatalf("query tool_events: %v", err)
	}
	if toolName != "Bash" || commandHead != "go" || isError != 1 {
		t.Errorf("tool_events row: %s %s %d", toolName, commandHead, isError)
	}
}

func TestSyncFilesPopulatesExitCode(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Q2 contract: extraction is the reader's job — it parses the FULL tool output
	// before truncation. SyncFiles persists the code it is handed and must NOT
	// re-derive one from Text, which is truncated and would lose codes past the cap.
	zero := 0
	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{
			// Carries a reader-extracted code: it must be persisted verbatim.
			{Ordinal: 0, Role: "assistant", Text: "exit code 0", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", ExitCode: &zero, ExtractionVersion: 1},
			// No reader-extracted code, but Text still looks like one: SyncFiles must
			// leave exit_code NULL rather than parsing the (possibly truncated) text.
			{Ordinal: 1, Role: "assistant", Text: "exit code 9", UUID: "u1#t1",
				Timestamp: "2026-01-01T00:00:01Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var exitCode *int64
	if err := db.db.QueryRow(`SELECT exit_code FROM tool_events WHERE message_uuid = 'u1#t0'`).
		Scan(&exitCode); err != nil {
		t.Fatalf("query: %v", err)
	}
	if exitCode == nil {
		t.Fatal("reader-extracted exit code was not persisted: got NULL")
	}
	if *exitCode != 0 {
		t.Errorf("exit_code = %d, want 0", *exitCode)
	}

	var unset *int64
	if err := db.db.QueryRow(`SELECT exit_code FROM tool_events WHERE message_uuid = 'u1#t1'`).
		Scan(&unset); err != nil {
		t.Fatalf("query ordinal 1: %v", err)
	}
	if unset != nil {
		t.Errorf("SyncFiles re-derived exit_code %d from Text; extraction belongs to the reader", *unset)
	}
}
