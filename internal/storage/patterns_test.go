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

func TestAggregateCommandsTrendMultiWeek(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Seed: commands across multiple weeks
	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			// Week 2026-W27
			{Ordinal: 0, Role: "assistant", Text: "test", UUID: "u1#t0",
				Timestamp: "2026-06-29T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
			// Week 2026-W28
			{Ordinal: 1, Role: "assistant", Text: "build", UUID: "u2#t0",
				Timestamp: "2026-07-07T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
			{Ordinal: 2, Role: "assistant", Text: "build", UUID: "u3#t0",
				Timestamp: "2026-07-08T00:00:00Z", ContentType: "tool",
				ToolName: "Edit", CommandHead: "file", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateCommandsTrend(AggregateOptions{Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("want 3 results, got %d", len(results))
	}
	// Should have weeks in the results
	hasWeek := false
	for _, r := range results {
		if r.Week != "" {
			hasWeek = true
			break
		}
	}
	if !hasWeek {
		t.Errorf("results should have non-empty weeks")
	}
}

func TestAggregateFailuresTrendMultiWeek(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Seed: failures across multiple weeks
	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			// Week 2026-W27
			{Ordinal: 0, Role: "assistant", Text: "error1", UUID: "u1#t0",
				Timestamp: "2026-06-29T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true), ExtractionVersion: 1},
			// Week 2026-W28
			{Ordinal: 1, Role: "assistant", Text: "error2", UUID: "u2#t0",
				Timestamp: "2026-07-07T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true), ExtractionVersion: 1},
			{Ordinal: 2, Role: "assistant", Text: "error3", UUID: "u3#t0",
				Timestamp: "2026-07-08T00:00:00Z", ContentType: "tool",
				ToolName: "Edit", CommandHead: "file", IsError: boolPtr(true), ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateFailuresTrend(AggregateOptions{Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("want 3 results, got %d", len(results))
	}
	// Check that results have week values
	for _, r := range results {
		if r.Week == "" {
			t.Errorf("result missing week: %+v", r)
		}
	}
}

func TestAggregateCommandsTrendProjectFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{
		{
			SourcePath: "/p1/s1.jsonl", Source: "session", Hash: "h1", Project: "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", Text: "test", UUID: "u1#t0",
					Timestamp: "2026-07-01T00:00:00Z", ContentType: "tool",
					ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
			},
		},
		{
			SourcePath: "/p2/s2.jsonl", Source: "session", Hash: "h2", Project: "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", Text: "test", UUID: "u2#t0",
					Timestamp: "2026-07-01T00:00:00Z", ContentType: "tool",
					ToolName: "Bash", CommandHead: "cargo", ExtractionVersion: 1},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateCommandsTrend(AggregateOptions{Project: "proj1", Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 || results[0].CommandHead != "go" {
		t.Errorf("project filter failed: want 1×go, got %+v", results)
	}
}

func TestAggregateFailuresTrendProjectFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{
		{
			SourcePath: "/p1/s1.jsonl", Source: "session", Hash: "h1", Project: "proj1",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", Text: "error", UUID: "u1#t0",
					Timestamp: "2026-07-01T00:00:00Z", ContentType: "tool",
					ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true), ExtractionVersion: 1},
			},
		},
		{
			SourcePath: "/p2/s2.jsonl", Source: "session", Hash: "h2", Project: "proj2",
			Messages: []IndexedMessage{
				{Ordinal: 0, Role: "assistant", Text: "error", UUID: "u2#t0",
					Timestamp: "2026-07-01T00:00:00Z", ContentType: "tool",
					ToolName: "Bash", CommandHead: "cargo", IsError: boolPtr(true), ExtractionVersion: 1},
			},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateFailuresTrend(AggregateOptions{Project: "proj1", Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 || results[0].ToolName != "Bash" {
		t.Errorf("project filter failed: want 1 Bash result, got %+v", results)
	}
}

func TestAggregateCommandsTrendEmpty(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Empty corpus
	var excluded int
	results, err := db.AggregateCommandsTrend(AggregateOptions{Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("want 0 results for empty corpus, got %d", len(results))
	}
}

func TestAggregateFailuresTrendEmpty(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	// Empty corpus
	var excluded int
	results, err := db.AggregateFailuresTrend(AggregateOptions{Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("want 0 results for empty corpus, got %d", len(results))
	}
}

func TestAggregateCommandsTrendTagFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "test", UUID: "u1#t0",
				Timestamp: "2026-07-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
		},
		Tags: []string{"debugging"},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateCommandsTrend(AggregateOptions{Tag: "debugging", Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 || results[0].CommandHead != "go" {
		t.Errorf("tag filter failed: want 1×go, got %+v", results)
	}
}

func TestAggregateFailuresTrendTagFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "error", UUID: "u1#t0",
				Timestamp: "2026-07-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
		},
		Tags: []string{"testing"},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateFailuresTrend(AggregateOptions{Tag: "testing", Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("tag filter failed: want 1 result, got %d", len(results))
	}
}

func TestAggregateCommandsTrendDateFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "test", UUID: "u1#t0",
				Timestamp: "2026-06-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "go", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "test", UUID: "u2#t0",
				Timestamp: "2026-07-15T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "build", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Manually insert a NULL-timestamp row to test exclusion counting
	if _, err := db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, content_type, extraction_version)
		VALUES ('session', '/p/s1.jsonl', 2, 'assistant', 'test', NULL, 'tool', 1)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, extraction_version)
		VALUES (NULL, '/p/s1.jsonl', 2, 'Bash', 'fmt', 1)
	`); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateCommandsTrend(AggregateOptions{StartDate: "2026-07-01T00:00:00Z", Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 || results[0].CommandHead != "build" {
		t.Errorf("date filter failed: want 1×build after filter, got %+v", results)
	}
	if excluded != 1 {
		t.Errorf("excluded count: want 1 NULL-timestamp row, got %d", excluded)
	}
}

func TestAggregateFailuresTrendDateFilter(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "error", UUID: "u1#t0",
				Timestamp: "2026-06-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "test", IsError: boolPtr(true), ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "error", UUID: "u2#t0",
				Timestamp: "2026-07-15T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "build", IsError: boolPtr(true), ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	// Manually insert NULL-timestamp rows to test exclusion counting
	// Row 2: is_error=1 with NULL timestamp (should be excluded and counted)
	if _, err := db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, content_type, extraction_version)
		VALUES ('session', '/p/s1.jsonl', 2, 'assistant', 'error', NULL, 'tool', 1)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, extraction_version)
		VALUES (NULL, '/p/s1.jsonl', 2, 'Bash', 'check', 1, 1)
	`); err != nil {
		t.Fatal(err)
	}
	// Row 3: is_error=0 (success) with NULL timestamp (should NOT be counted, only is_error=1 counted)
	if _, err := db.db.Exec(`
		INSERT INTO search_items (source, source_path, ordinal, role, text, timestamp, content_type, extraction_version)
		VALUES ('session', '/p/s1.jsonl', 3, 'assistant', 'success', NULL, 'tool', 1)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`
		INSERT INTO tool_events (message_uuid, source_path, ordinal, tool_name, command_head, is_error, extraction_version)
		VALUES (NULL, '/p/s1.jsonl', 3, 'Bash', 'ok', 0, 1)
	`); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateFailuresTrend(AggregateOptions{EndDate: "2026-07-01T00:00:00Z", Limit: 10}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 || results[0].ToolName != "Bash" {
		t.Errorf("date filter failed: want 1 Bash result before 2026-07-01, got %+v", results)
	}
	if excluded != 1 {
		t.Errorf("excluded count: want 1 is_error=1 NULL-timestamp row (success should not count), got %d", excluded)
	}
}

func TestAggregateCommandsTrendWithOffset(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s1.jsonl", Source: "session", Hash: "h1", Project: "p1",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "test", UUID: "u1#t0",
				Timestamp: "2026-07-01T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "a", ExtractionVersion: 1},
			{Ordinal: 1, Role: "assistant", Text: "test", UUID: "u2#t0",
				Timestamp: "2026-07-02T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "b", ExtractionVersion: 1},
			{Ordinal: 2, Role: "assistant", Text: "test", UUID: "u3#t0",
				Timestamp: "2026-07-03T00:00:00Z", ContentType: "tool",
				ToolName: "Bash", CommandHead: "c", ExtractionVersion: 1},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	var excluded int
	results, err := db.AggregateCommandsTrend(AggregateOptions{Limit: 1, Offset: 1}, &excluded)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("offset should limit results: got %d", len(results))
	}
}

// Helper function for test assertions
func contains(s, substr string) bool {
	for i := 0; i < len(s); i++ {
		if len(s)-i >= len(substr) {
			match := true
			for j := 0; j < len(substr); j++ {
				if s[i+j] != substr[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
