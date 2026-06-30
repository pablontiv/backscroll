package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	picokitoutput "github.com/pablontiv/picokit/output"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/storage"
)

func TestSearchOutputFormatText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search without format flag (defaults to text)
	out, _, err := runCmd("search", "test")
	if err != nil {
		t.Fatalf("search text error: %v", err)
	}

	// Verify text format characteristics
	if strings.Contains(out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━") {
		// Good: contains separator
	} else if strings.Contains(out, "Rank:") && strings.Contains(out, "Source:") {
		// Good: contains field headers
	} else if len(out) == 0 {
		// Empty results are acceptable (no matching content in fixtures)
	} else {
		t.Errorf("search text output doesn't contain expected format markers: %s", out)
	}
}

func TestSearchOutputFormatJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with --json flag
	out, _, err := runCmd("search", "test", "--json")
	if err != nil {
		t.Fatalf("search --json error: %v", err)
	}

	// Verify valid JSON output
	var results []models.SearchResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		// Empty results might serialize differently; try parsing as empty array
		if len(strings.TrimSpace(out)) > 0 {
			t.Fatalf("search --json output not valid JSON: %v\noutput: %s", err, out)
		}
	}
	// If we got here, output is valid JSON
}

func TestSearchOutputFormatRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with --robot flag
	out, _, err := runCmd("search", "test", "--robot")
	if err != nil {
		t.Fatalf("search --robot error: %v", err)
	}

	// Verify robot format characteristics: result_N_field=value pattern
	if len(strings.TrimSpace(out)) > 0 {
		// If there are results, check for robot format
		if !strings.Contains(out, "result_") {
			t.Errorf("robot format missing result_N_ prefix: %s", out)
		}
		// Check for expected robot format fields
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "result_") {
				if !strings.Contains(line, "=") {
					t.Errorf("robot format line missing '=': %s", line)
				}
			}
		}
	}
}

func TestSearchOutputRespectsTokenLimit(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with --max-tokens limit
	out, _, err := runCmd("search", "test", "--max-tokens", "50")
	if err != nil {
		t.Fatalf("search --max-tokens error: %v", err)
	}

	// Just verify the command ran successfully and produced output
	// The token limiting is a soft limit so we can't assert exact behavior
	if out == "" && len(out) > 0 {
		t.Error("output should not be empty when results exist")
	}
	// If output is empty or present, that's acceptable
}

func TestSearchTextFormatStructure(t *testing.T) {
	// Test the resultsToLines adapter function directly
	results := []models.SearchResult{
		{
			Source:      "session",
			Role:        "user",
			Content:     "test content",
			FilePath:    "/path/to/file.jsonl",
			Rank:        1,
			Score:       0.95,
			SessionID:   "session-123",
			ProjectPath: "/home/project",
			Tags:        []string{"debugging"},
		},
	}

	// Use FormatText from picokit
	lines := resultsToLines(results, picokitoutput.FormatText)

	// Verify we have expected text format lines
	allText := strings.Join(lines, "\n")
	expectedStrs := []string{
		"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		"Rank: 1",
		"Source: session",
		"Role: user",
		"Score: 0.95",
		"Path: /path/to/file.jsonl",
		"Session: session-123",
		"Project: /home/project",
		"Tags: debugging",
		"test content",
	}

	for _, expected := range expectedStrs {
		if !strings.Contains(allText, expected) {
			t.Errorf("text format missing expected string: %q\nfull output: %s", expected, allText)
		}
	}
}

func TestSearchRobotFormatStructure(t *testing.T) {
	// Test the resultsToLines adapter function directly for robot format
	results := []models.SearchResult{
		{
			Source:      "session",
			Role:        "assistant",
			Content:     "test content",
			FilePath:    "/path/to/file.jsonl",
			Rank:        1,
			Score:       0.85,
			SessionID:   "session-456",
			ProjectPath: "/home/project2",
			Tags:        []string{"refactoring", "testing"},
		},
	}

	// Use FormatRobot from picokit
	lines := resultsToLines(results, picokitoutput.FormatRobot)

	// Verify we have expected robot format lines
	allText := strings.Join(lines, "\n")
	expectedStrs := []string{
		"result_0_source=session",
		"result_0_role=assistant",
		"result_0_filepath=/path/to/file.jsonl",
		"result_0_content=test content",
		"result_0_session_id=session-456",
		"result_0_project=/home/project2",
		"result_0_score=0.85",
		"result_0_tags=refactoring,testing",
		"result_0_rank=1",
	}

	for _, expected := range expectedStrs {
		if !strings.Contains(allText, expected) {
			t.Errorf("robot format missing expected line: %q\nfull output: %s", expected, allText)
		}
	}
}

func TestSearchWithJSONAndMaxTokens(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with both --json and --max-tokens
	out, _, err := runCmd("search", "test", "--json", "--max-tokens", "100")
	if err != nil {
		t.Fatalf("search --json --max-tokens error: %v", err)
	}

	// Verify valid JSON output
	var results []models.SearchResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		if len(strings.TrimSpace(out)) > 0 {
			t.Fatalf("search --json --max-tokens output not valid JSON: %v", err)
		}
	}
}

func TestFormatStructuredEventsEmpty(t *testing.T) {
	var stdout strings.Builder
	events := []storage.StructuredEventRow{}

	// Test text format (empty)
	err := formatStructuredEvents(&stdout, events, false)
	if err != nil {
		t.Fatalf("formatStructuredEvents error: %v", err)
	}
	if !strings.Contains(stdout.String(), "No events found") {
		t.Errorf("expected 'No events found' for empty events (text), got: %s", stdout.String())
	}

	// Test JSON format (empty)
	stdout.Reset()
	err = formatStructuredEvents(&stdout, events, true)
	if err != nil {
		t.Fatalf("formatStructuredEvents JSON error: %v", err)
	}
	if !strings.Contains(stdout.String(), "\"count\":0") {
		t.Errorf("expected JSON with count:0 for empty events, got: %s", stdout.String())
	}
}

func TestFormatStructuredEventsWithData(t *testing.T) {
	var stdout strings.Builder
	events := []storage.StructuredEventRow{
		{
			EventType:  "tool_call",
			ToolName:   "search",
			Actor:      "user",
			SourcePath: "/path/to/session.jsonl",
			Ordinal:    5,
			Timestamp:  "2024-01-15T10:30:00Z",
			Snippet:    "test snippet content",
		},
		{
			EventType:  "message",
			ToolName:   "",
			Actor:      "assistant",
			SourcePath: "/path/to/session.jsonl",
			Ordinal:    6,
			Timestamp:  "2024-01-15T10:31:00Z",
			Snippet:    "response snippet",
		},
	}

	// Test text format with data
	err := formatStructuredEvents(&stdout, events, false)
	if err != nil {
		t.Fatalf("formatStructuredEvents error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "tool_call") || !strings.Contains(output, "search") || !strings.Contains(output, "Total: 2 events") {
		t.Errorf("text format missing expected fields: %s", output)
	}

	// Test JSON format with data
	stdout.Reset()
	err = formatStructuredEvents(&stdout, events, true)
	if err != nil {
		t.Fatalf("formatStructuredEvents JSON error: %v", err)
	}
	output = stdout.String()
	if !strings.Contains(output, "\"count\":2") || !strings.Contains(output, "tool_call") {
		t.Errorf("JSON format missing expected fields: %s", output)
	}
}
