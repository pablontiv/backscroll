package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

// seedToolEvents plants a small deterministic tool-event corpus for census tests.
func seedToolEvents(t *testing.T) {
	t.Helper()
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	msgs := []storage.IndexedMessage{
		{Ordinal: 0, UUID: "cv1", Role: "assistant", Text: "Bash command=go test", ContentType: "tool",
			ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true), Timestamp: "2026-01-05T00:00:00Z", ExtractionVersion: 1},
		{Ordinal: 1, UUID: "cv2", Role: "assistant", Text: "Read file_path=/a.go", ContentType: "tool",
			ToolName: "Read", Timestamp: "2026-01-12T00:00:00Z", ExtractionVersion: 1},
		{Ordinal: 2, UUID: "cv3", Role: "assistant", Text: "Bash command=go vet", ContentType: "tool",
			ToolName: "Bash", CommandHead: "go", IsError: boolPtr(false), Timestamp: "2026-01-12T00:00:01Z", ExtractionVersion: 1},
	}
	files := []storage.IndexedFile{{SourcePath: "/cov/s.jsonl", Source: "session", Hash: "hcov", Project: "covproj", Messages: msgs}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
}

func TestPatternsCommandsJSONWithData(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Bash") {
		t.Errorf("json output missing seeded tool: %q", stdout)
	}
}

func TestPatternsFailuresRobotWithData(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--robot", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "result_") {
		t.Errorf("robot output missing result lines: %q", stdout)
	}
}

func TestPatternsSequencesRobotSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("patterns", "--kind", "sequences", "--robot", "--min-support", "1", "--min-length", "2", "--indexed-only"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestPatternsCorrectionsJSONEmpty(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t) // creates the DB; corpus has no correction signals
	if _, _, err := runCmd("patterns", "--kind", "corrections", "--json", "--all-projects", "--indexed-only"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestPatternsNegativeLimitRejected(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	if _, _, err := runCmd("patterns", "--kind", "commands", "--limit", "-3"); err == nil {
		t.Fatal("negative limit must be rejected before DB open")
	}
}

func TestPatternsProjectAllProjectsConflict(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	if _, _, err := runCmd("patterns", "--kind", "commands", "--project", "x", "--all-projects"); err == nil {
		t.Fatal("project + all-projects must be rejected")
	}
}

func TestPatternsCommandsRobotSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--robot", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "result_") {
		t.Errorf("robot output missing result lines: %q", stdout)
	}
}

func TestPatternsFailuresJSONSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Bash") {
		t.Errorf("failures json missing seeded tool: %q", stdout)
	}
}

func TestPatternsFailuresTextWithTag(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("patterns", "--kind", "failures", "--tag", "debugging", "--all-projects", "--indexed-only"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestPatternsTemplatesMinSupportSeeded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	if _, _, err := runCmd("patterns", "--kind", "templates", "--min-support", "1", "--all-projects", "--indexed-only"); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestPatternsFailuresTextFormatNullExitCode tests that NULL exit_code prints "?" in text format
func TestPatternsFailuresTextFormatNullExitCode(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Check that text output uses "exit_code=?" for NULL values
	if !strings.Contains(stdout, "exit_code=?") {
		t.Errorf("text format must show exit_code=? for NULL; output:\n%s", stdout)
	}
	// Verify that <nil> doesn't appear in output
	if strings.Contains(stdout, "<nil>") {
		t.Errorf("text format must not show <nil>; output:\n%s", stdout)
	}
}

// TestPatternsFailuresRobotFormatNullExitCode tests that NULL exit_code prints "?" in robot format
func TestPatternsFailuresRobotFormatNullExitCode(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--robot", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Check that robot output uses exit_code=? for NULL values
	if !strings.Contains(stdout, "exit_code=?") {
		t.Errorf("robot format must show exit_code=? for NULL; output:\n%s", stdout)
	}
	// Verify that "null" doesn't appear in exit_code lines
	if strings.Contains(stdout, "exit_code=null") {
		t.Errorf("robot format must not show exit_code=null; output:\n%s", stdout)
	}
}

// seedTrendData creates multi-week tool events for trend testing
func seedTrendData(t *testing.T) {
	t.Helper()
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	msgs := []storage.IndexedMessage{
		// Week 2026-W27 (early June)
		{Ordinal: 0, UUID: "t1", Role: "assistant", Text: "Bash command=go test", ContentType: "tool",
			ToolName: "Bash", CommandHead: "go", IsError: boolPtr(false), Timestamp: "2026-06-29T00:00:00Z", ExtractionVersion: 1},
		{Ordinal: 1, UUID: "t2", Role: "assistant", Text: "Bash command=git push exit code: 1", ContentType: "tool",
			ToolName: "Bash", CommandHead: "git", IsError: boolPtr(true), Timestamp: "2026-06-30T00:00:00Z", ExtractionVersion: 1},
		// Week 2026-W28 (early July)
		{Ordinal: 2, UUID: "t3", Role: "assistant", Text: "Read file=/main.go", ContentType: "tool",
			ToolName: "Read", CommandHead: "file", IsError: boolPtr(false), Timestamp: "2026-07-07T00:00:00Z", ExtractionVersion: 1},
		{Ordinal: 3, UUID: "t4", Role: "assistant", Text: "Edit file=/main.go exit code: 127", ContentType: "tool",
			ToolName: "Edit", CommandHead: "file", IsError: boolPtr(true), Timestamp: "2026-07-08T00:00:00Z", ExtractionVersion: 1},
		{Ordinal: 4, UUID: "t5", Role: "assistant", Text: "Bash command=go test exit code: 1", ContentType: "tool",
			ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true), Timestamp: "2026-07-09T00:00:00Z", ExtractionVersion: 1},
	}
	files := []storage.IndexedFile{{SourcePath: "/trend/s.jsonl", Source: "session", Hash: "htrend", Project: "trendproj", Messages: msgs}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
}

// Trend validation tests
func TestPatternsTrendWithSequencesRejectEarly(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	_, _, err := runCmd("patterns", "--kind", "sequences", "--trend")
	if err == nil {
		t.Fatal("--trend with --kind sequences must fail early before DB open")
	}
	if !strings.Contains(err.Error(), "trend") || !strings.Contains(err.Error(), "sequences") {
		t.Errorf("error message must mention trend and sequences: %v", err)
	}
}

func TestPatternsTrendWithTemplatesRejectEarly(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	_, _, err := runCmd("patterns", "--kind", "templates", "--trend")
	if err == nil {
		t.Fatal("--trend with --kind templates must fail early before DB open")
	}
	if !strings.Contains(err.Error(), "trend") || !strings.Contains(err.Error(), "templates") {
		t.Errorf("error message must mention trend and templates: %v", err)
	}
}

func TestPatternsTrendWithCorrectionsRejectEarly(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	_, _, err := runCmd("patterns", "--kind", "corrections", "--trend")
	if err == nil {
		t.Fatal("--trend with --kind corrections must fail early before DB open")
	}
	if !strings.Contains(err.Error(), "trend") {
		t.Errorf("error message must mention trend: %v", err)
	}
}

// Trend text output (commands)
func TestPatternsTrendCommandsText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedTrendData(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--trend", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Trends in Commands") {
		t.Errorf("text output must contain header; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "2026-W") {
		t.Errorf("text output must contain week; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Bash") {
		t.Errorf("text output must contain tool name; got:\n%s", stdout)
	}
}

// Trend JSON output (commands)
func TestPatternsTrendCommandsJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedTrendData(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--trend", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		t.Fatalf("JSON parse error: %v; output:\n%s", err, stdout)
	}
	if data["kind"] != "commands" {
		t.Errorf("JSON kind must be 'commands'; got %v", data["kind"])
	}
	if _, ok := data["trends"]; !ok {
		t.Errorf("JSON must contain 'trends' key; got keys: %v", data)
	}
}

// Trend robot output (commands)
func TestPatternsTrendCommandsRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedTrendData(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--trend", "--robot", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "week_") {
		t.Errorf("robot output must contain week_N lines; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "result_") {
		t.Errorf("robot output must contain result lines; got:\n%s", stdout)
	}
}

// Trend text output (failures)
func TestPatternsTrendFailuresText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedTrendData(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--trend", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Trends in Failures") {
		t.Errorf("text output must contain header; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "2026-W") {
		t.Errorf("text output must contain week; got:\n%s", stdout)
	}
}

// Trend JSON output (failures)
func TestPatternsTrendFailuresJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedTrendData(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--trend", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		t.Fatalf("JSON parse error: %v; output:\n%s", err, stdout)
	}
	if data["kind"] != "failures" {
		t.Errorf("JSON kind must be 'failures'; got %v", data["kind"])
	}
	if _, ok := data["trends"]; !ok {
		t.Errorf("JSON must contain 'trends' key; got keys: %v", data)
	}
}

// Trend robot output (failures)
func TestPatternsTrendFailuresRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedTrendData(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--trend", "--robot", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "week_") {
		t.Errorf("robot output must contain week_N lines; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "result_") {
		t.Errorf("robot output must contain result lines; got:\n%s", stdout)
	}
}

// Text output for non-trend (commands)
func TestPatternsCommandsText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Top Commands") {
		t.Errorf("text output must contain header; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Bash") {
		t.Errorf("text output must contain tool name; got:\n%s", stdout)
	}
}

// Text output for non-trend (failures)
func TestPatternsFailuresText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "failures", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Failure Patterns") {
		t.Errorf("text output must contain header; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Signalled events") {
		t.Errorf("text output must contain coverage info; got:\n%s", stdout)
	}
}

// Text output (templates)
func TestPatternsTemplatesText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "templates", "--min-support", "1", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "Found") && !strings.Contains(stdout, "templates") {
		t.Errorf("text output must mention templates; got:\n%s", stdout)
	}
}

// Text output (corrections)
func TestPatternsCorrectionsText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "corrections", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Even with no corrections, output should be valid text or JSON-based
	if len(stdout) == 0 {
		t.Errorf("output should not be empty; got:\n%s", stdout)
	}
}

// Text output (sequences)
func TestPatternsSequencesText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "sequences", "--min-support", "1", "--min-length", "2", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Output should be valid (either found patterns or "No patterns found")
	if len(stdout) == 0 {
		t.Errorf("output should not be empty; got:\n%s", stdout)
	}
}

// Zero-result tests
func TestPatternsCommandsZeroResultText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "commands", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "No patterns found") {
		t.Errorf("text output must indicate no patterns; got:\n%s", stdout)
	}
}

func TestPatternsFailuresZeroResultText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "failures", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "No failure patterns found") {
		t.Errorf("text output must indicate no patterns; got:\n%s", stdout)
	}
}

func TestPatternsTemplatesZeroResultText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "templates", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "No templates found") {
		t.Errorf("text output must indicate no templates; got:\n%s", stdout)
	}
}

func TestPatternsCorrectionsZeroResultText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "corrections", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "No correction candidates found") {
		t.Errorf("text output must indicate no corrections; got:\n%s", stdout)
	}
}

func TestPatternsSequencesZeroResultText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "sequences", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "No patterns found") {
		t.Errorf("text output must indicate no patterns; got:\n%s", stdout)
	}
}

// Edge cases
func TestPatternsCommandsLimitZero(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--limit", "0", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(stdout) == 0 {
		t.Errorf("output should not be empty with --limit 0; got:\n%s", stdout)
	}
}

func TestPatternsCommandsOffsetBeyond(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--offset", "999", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "No patterns found") {
		t.Errorf("text output should indicate no patterns at high offset; got:\n%s", stdout)
	}
}

func TestPatternsBatchAlias(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	// --batch should alias to --limit for corrections
	stdout, _, err := runCmd("patterns", "--kind", "corrections", "--batch", "10", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(stdout) == 0 {
		t.Errorf("output should not be empty with --batch; got:\n%s", stdout)
	}
}

// Additional JSON output tests for zero-result cases
func TestPatternsCommandsZeroResultJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "commands", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "{") || !strings.Contains(stdout, "}") {
		t.Errorf("JSON output should be valid; got:\n%s", stdout)
	}
}

func TestPatternsFailuresZeroResultJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "failures", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "{") || !strings.Contains(stdout, "}") {
		t.Errorf("JSON output should be valid; got:\n%s", stdout)
	}
}

// Test JSON output for non-zero results
func TestPatternsCommandsJSONOutput(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "commands", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &data); err != nil {
		t.Fatalf("JSON parse error: %v; output:\n%s", err, stdout)
	}
	if data["kind"] != "commands" {
		t.Errorf("JSON kind should be 'commands'")
	}
}

// Test robot output for all kinds
func TestPatternsTemplatesRobotOutput(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "templates", "--robot", "--min-support", "1", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "***") {
		t.Errorf("robot output should contain *** markers; got:\n%s", stdout)
	}
}

func TestPatternsSequencesRobotOutput(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	stdout, _, err := runCmd("patterns", "--kind", "sequences", "--robot", "--min-support", "1", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Even empty sequences should produce robot format
	if len(stdout) == 0 {
		t.Errorf("robot output should not be empty; got:\n%s", stdout)
	}
}

func TestPatternsCorrectionsRobotOutput(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	seedToolEvents(t)
	_, _, err := runCmd("patterns", "--kind", "corrections", "--robot", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// Just verify the command succeeds (output may be empty with no corrections)
}

// Test JSON output for zero results
func TestPatternsTemplatesZeroResultJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "templates", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "{") || !strings.Contains(stdout, "}") {
		t.Errorf("JSON output should be valid; got:\n%s", stdout)
	}
}

func TestPatternsSequencesZeroResultJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "sequences", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "{") || !strings.Contains(stdout, "}") {
		t.Errorf("JSON output should be valid; got:\n%s", stdout)
	}
}

func TestPatternsCorrectionsZeroResultJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	// Create DB with no data
	db, err := storage.Open(os.Getenv("BACKSCROLL_DATABASE_PATH"))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	stdout, _, err := runCmd("patterns", "--kind", "corrections", "--json", "--all-projects", "--indexed-only")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(stdout, "{") || !strings.Contains(stdout, "}") {
		t.Errorf("JSON output should be valid; got:\n%s", stdout)
	}
}
