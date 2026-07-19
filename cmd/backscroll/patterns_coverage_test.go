package main

import (
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
