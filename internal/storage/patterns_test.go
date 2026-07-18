package storage

import (
	"path/filepath"
	"testing"
)

func TestAggregateCommandsTopPairs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Seed: 3x Bash/test, 2x Bash/fmt, 1x Bash/vet
	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "test output", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "test output", UUID: "u2#t0",
				Timestamp: "2026-01-02T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", ExtractionVersion: 1},
			{Ordinal: 2, Role: "assistant", Text: "test output", UUID: "u3#t0",
				Timestamp: "2026-01-03T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", ExtractionVersion: 1},
			{Ordinal: 3, Role: "assistant", Text: "fmt output", UUID: "u4#t0",
				Timestamp: "2026-01-04T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "fmt", ExtractionVersion: 1},
			{Ordinal: 4, Role: "assistant", Text: "fmt output", UUID: "u5#t0",
				Timestamp: "2026-01-05T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "fmt", ExtractionVersion: 1},
			{Ordinal: 5, Role: "assistant", Text: "vet output", UUID: "u6#t0",
				Timestamp: "2026-01-06T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "vet", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	results, err := db.AggregateCommands(AggregateOptions{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("want 3 results, got %d: %+v", len(results), results)
	}
	if results[0].Count != 3 || results[0].CommandHead != "test" {
		t.Errorf("rank 1: want 3×test, got %d×%s", results[0].Count, results[0].CommandHead)
	}
	if results[1].Count != 2 {
		t.Errorf("rank 2: want count=2, got %d", results[1].Count)
	}
}

func TestAggregateFailuresWithCoverage(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Seed: 2 failures (is_error=true), 1 success (is_error=false), 2 unknown (is_error=NULL)
	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "FAIL", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "FAIL", UUID: "u2#t0",
				Timestamp: "2026-01-02T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
			{Ordinal: 2, Role: "assistant", Text: "PASS", UUID: "u3#t0",
				Timestamp: "2026-01-03T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(false), ExtractionVersion: 1},
			{Ordinal: 3, Role: "assistant", Text: "no signal", UUID: "u4#t0",
				Timestamp: "2026-01-04T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: nil, ExtractionVersion: 1},
			{Ordinal: 4, Role: "assistant", Text: "no signal", UUID: "u5#t0",
				Timestamp: "2026-01-05T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: nil, ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	results, err := db.AggregateFailures(AggregateOptions{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("want 1 result (only is_error=true), got %d", len(results))
	}
	if results[0].Count != 2 {
		t.Errorf("failure count: want 2, got %d", results[0].Count)
	}
	// Coverage: 5 total events - 2 with NULL is_error = 3 events with signal
	if results[0].SignalledEvents != 3 {
		t.Errorf("signalled_events: want 3, got %d", results[0].SignalledEvents)
	}
}

func TestAggregateCommandsProjectFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{
		{
			SourcePath: "/p1/s1.jsonl", Source: "session", Hash: "h1", Project: "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", Text: "out", UUID: "u1#t0",
					Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
					ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
			},
		},
		{
			SourcePath: "/p2/s2.jsonl", Source: "session", Hash: "h2", Project: "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", Text: "out", UUID: "u2#t0",
					Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
					ToolName: "Bash", CommandHead: "cargo", ExtractionVersion: 1},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	results, err := db.AggregateCommands(AggregateOptions{Project: "proj1", Limit: 10})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 || results[0].CommandHead != "go" {
		t.Errorf("project filter failed: want 1×go, got %+v", results)
	}
}

func TestAggregateCommandsWithOffset(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Seed: 3 different commands
	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "test", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "cmd1", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "test", UUID: "u2#t0",
				Timestamp: "2026-01-02T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "cmd1", ExtractionVersion: 1},
			{Ordinal: 2, Role: "assistant", Text: "test", UUID: "u3#t0",
				Timestamp: "2026-01-03T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "cmd2", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	results, err := db.AggregateCommands(AggregateOptions{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 || results[0].CommandHead != "cmd2" {
		t.Errorf("offset filter failed: want cmd2, got %+v", results)
	}
}

func TestAggregateFailuresWithMultipleTool(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Seed: failures from different tools
	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "exit code 1", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "error", UUID: "u2#t0",
				Timestamp: "2026-01-02T00:00:00Z", ContentType: "tool",
				ToolName: "Edit", CommandHead: "apply", IsError: boolPtr(true), ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	results, err := db.AggregateFailures(AggregateOptions{Limit: 10})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("want 2 results (2 tools), got %d", len(results))
	}
}

func TestAggregateFailuresWithTagFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "error", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
		},
		Tags: []string{"debugging"},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	results, err := db.AggregateFailures(AggregateOptions{Tag: "debugging", Limit: 10})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("want 1 result with tag filter, got %d", len(results))
	}
}

func TestAggregateFailuresWithDateFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "error", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "error", UUID: "u2#t0",
				Timestamp: "2026-01-15T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Filter to only after 2026-01-10
	results, err := db.AggregateFailures(AggregateOptions{StartDate: "2026-01-10T00:00:00Z", Limit: 10})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("want 1 result after date filter, got %d", len(results))
	}
}

func TestAggregateFailuresEmptyCorpus(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Sync only successful runs, no failures
	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "success", UUID: "u1#t0",
				Timestamp: "2026-01-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(false), ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	results, err := db.AggregateFailures(AggregateOptions{Limit: 10})
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("want 0 results for all-success corpus, got %d", len(results))
	}
}
