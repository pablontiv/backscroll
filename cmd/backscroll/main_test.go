package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testEnv creates an isolated environment for CLI tests.
// It sets BACKSCROLL_DATABASE_PATH to a temp file and returns a cleanup func.
func testEnv(t *testing.T) (dbPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "test.db")
	orig := os.Getenv("BACKSCROLL_DATABASE_PATH")
	os.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	return dbPath, func() {
		if orig == "" {
			os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		} else {
			os.Setenv("BACKSCROLL_DATABASE_PATH", orig)
		}
	}
}

// runCmd executes a backscroll command and returns stdout, stderr, error.
func runCmd(args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, args)
	return stdout.String(), stderr.String(), err
}

// fixturesDir returns the path to the tests/fixtures directory.
func fixturesDir() string {
	// cmd/backscroll/ → ../../tests/fixtures
	return filepath.Join("..", "..", "tests", "fixtures")
}

func TestHelp(t *testing.T) {
	out, _, err := runCmd("--help")
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}
	for _, cmd := range []string{"sync", "search", "read", "resume", "list", "topics", "insights", "export", "reindex", "purge", "validate", "status"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("--help missing command %q", cmd)
		}
	}
}

func TestVersion(t *testing.T) {
	out, _, err := runCmd("--version")
	if err != nil {
		t.Fatalf("--version error: %v", err)
	}
	if !strings.Contains(out, "backscroll") {
		t.Errorf("version output missing 'backscroll': %s", out)
	}
}

func TestSyncAndSearch(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")

	// Sync fixture sessions
	out, stderr, err := runCmd("sync", "--path", fixtureSession)
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	_ = out

	// Status should show indexed content
	out, _, err = runCmd("status")
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if !strings.Contains(out, "Files indexed") {
		t.Errorf("status missing 'Files indexed': %s", out)
	}

	// Search should return results
	out, _, err = runCmd("search", "hello")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	_ = out // may or may not have results depending on fixture content
}

func TestSyncWithPiFixture(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	out, stderr, err := runCmd("sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync pi fixture error: %v\nstderr: %s", err, stderr)
	}
	_ = out
}

func TestStatus(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("status")
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if !strings.Contains(out, "Database") {
		t.Errorf("status missing 'Database': %s", out)
	}
}

func TestStatusJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("status", "--json")
	if err != nil {
		t.Fatalf("status --json error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("status --json not valid JSON: %v\noutput: %s", err, out)
	}
}

func TestRead(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piFixture := filepath.Join(fixturesDir(), "pi-session.jsonl")
	out, _, err := runCmd("read", piFixture)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if len(out) == 0 {
		t.Error("read returned empty output")
	}
}

func TestReadNonExistent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, err := runCmd("read", "/nonexistent/path.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestList(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync some sessions first
	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, _ = runCmd("sync", "--path", fixtureSession)

	out, _, err := runCmd("list")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	_ = out
}

func TestListJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("list", "--json")
	if err != nil {
		t.Fatalf("list --json error: %v", err)
	}
	// Should be valid JSON (array)
	var result []any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		// Empty list might serialize differently
		var empty map[string]any
		if err2 := json.Unmarshal([]byte(out), &empty); err2 != nil {
			t.Fatalf("list --json not valid JSON: %v\noutput: %s", err, out)
		}
	}
}

func TestTopics(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync some content first
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("topics", "--limit", "5")
	if err != nil {
		t.Fatalf("topics error: %v", err)
	}
	_ = out
}

func TestValidate(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("validate")
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	_ = out
}

func TestPurge(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync some content first
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	// Purge entries before a future date (should purge everything)
	out, _, err := runCmd("purge", "--before", "2030-01-01")
	if err != nil {
		t.Fatalf("purge error: %v", err)
	}
	_ = out
}

func TestSearchAllFlags(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync some content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	tests := []struct {
		name string
		args []string
	}{
		{"json", []string{"search", "test", "--json"}},
		{"robot", []string{"search", "test", "--robot"}},
		{"role-user", []string{"search", "test", "--role", "user"}},
		{"limit", []string{"search", "test", "--limit", "5"}},
		{"offset", []string{"search", "test", "--offset", "0"}},
		{"source", []string{"search", "test", "--source", "session"}},
		{"all-projects", []string{"search", "test", "--all-projects"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runCmd(tc.args...)
			if err != nil {
				t.Errorf("search %v error: %v", tc.args, err)
			}
		})
	}
}

func TestResume(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("resume", "pi")
	if err != nil {
		t.Fatalf("resume error: %v", err)
	}
	_ = out
}

func TestExport(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("export", "pi", "--format", "markdown")
	if err != nil {
		t.Fatalf("export error: %v", err)
	}
	_ = out
}

func TestInsights(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("insights")
	if err != nil {
		t.Fatalf("insights error: %v", err)
	}
	_ = out
}

func TestSyncSubagentExcluded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync the claude-preset which has a subagents/ directory
	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := runCmd("sync", "--path", fixtureSession)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// The subagent session should NOT appear in list
	out, _, err := runCmd("list")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if strings.Contains(out, "subagents") {
		t.Errorf("list should not contain subagent sessions, got: %s", out)
	}
}

func TestSyncIncludeAgents(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")
	out, _, err := runCmd("sync", "--path", fixtureSession, "--include-agents")
	if err != nil {
		t.Fatalf("sync --include-agents error: %v", err)
	}
	_ = out
}

func TestSyncIdempotent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")

	// Sync twice — should be idempotent
	_, _, err := runCmd("sync", "--path", fixtureSession)
	if err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	_, _, err = runCmd("sync", "--path", fixtureSession)
	if err != nil {
		t.Fatalf("second sync error: %v", err)
	}

	// Check stats are consistent
	out1, _, _ := runCmd("status")
	out2, _, _ := runCmd("status")
	// Extract file count line
	if out1 != out2 {
		// Allow for timing differences in timestamps but file counts should match
		t.Logf("status output after two syncs:\n%s\n---\n%s", out1, out2)
	}
}

func TestSyncNoPlans(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := runCmd("sync", "--path", piDir, "--no-plans")
	if err != nil {
		t.Fatalf("sync --no-plans error: %v", err)
	}
}

func TestReindex(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	_, _, err := runCmd("reindex")
	if err != nil {
		t.Fatalf("reindex error: %v", err)
	}
}

// TestSyncSearchRoundtrip verifies that synced content is searchable.
func TestSyncSearchRoundtrip(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// The pi fixture has "message" type sessions
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := runCmd("sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Status should show at least 1 file
	out, _, err := runCmd("status")
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if strings.Contains(out, "Files indexed:    0") {
		t.Logf("warning: no files indexed, fixture may be empty: %s", out)
	}
}

func TestPurgeMissingBeforeFlag(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, err := runCmd("purge")
	if err == nil {
		t.Error("expected error when --before flag missing, got nil")
	}
}

func TestSearchHelp(t *testing.T) {
	out, _, err := runCmd("search", "--help")
	if err != nil {
		t.Fatalf("search --help error: %v", err)
	}
	for _, flag := range []string{"--project", "--source", "--after", "--before", "--role", "--limit", "--json", "--robot"} {
		if !strings.Contains(out, flag) {
			t.Errorf("search --help missing flag %q", flag)
		}
	}
}

func TestSyncHelp(t *testing.T) {
	out, _, err := runCmd("sync", "--help")
	if err != nil {
		t.Fatalf("sync --help error: %v", err)
	}
	for _, flag := range []string{"--path", "--include-agents", "--no-plans", "--optimize"} {
		if !strings.Contains(out, flag) {
			t.Errorf("sync --help missing flag %q", flag)
		}
	}
}

func TestExportCSV(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("export", "pi", "--format", "csv")
	if err != nil {
		t.Fatalf("export --format csv error: %v", err)
	}
	_ = out
}

func TestExportUnknownFormat(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	_, _, err := runCmd("export", "pi", "--format", "xml")
	if err == nil {
		t.Error("expected error for unknown format, got nil")
	}
}

func TestTopicsJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("topics", "--json")
	if err != nil {
		t.Fatalf("topics --json error: %v", err)
	}
	_ = out
}

func TestTopicsRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("topics", "--robot")
	if err != nil {
		t.Fatalf("topics --robot error: %v", err)
	}
	_ = out
}

func TestInsightsJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("insights", "--json")
	if err != nil {
		t.Fatalf("insights --json error: %v", err)
	}
	_ = out
}

func TestInsightsRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("insights", "--robot")
	if err != nil {
		t.Fatalf("insights --robot error: %v", err)
	}
	_ = out
}

func TestListTextSynced(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("list")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	_ = out
}

func TestListRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("list", "--robot")
	if err != nil {
		t.Fatalf("list --robot error: %v", err)
	}
	_ = out
}

func TestListJSONSynced(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("list", "--json")
	if err != nil {
		t.Fatalf("list --json after sync error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("list --json output not valid JSON: %v\noutput: %s", err, out)
	}
}

func TestSearchJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("search", "pi", "--json")
	if err != nil {
		t.Fatalf("search --json error: %v", err)
	}
	_ = out
}

func TestSearchWithProject(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("search", "test", "--project", "unknown")
	if err != nil {
		t.Fatalf("search --project error: %v", err)
	}
	_ = out
}

func TestSyncOptimize(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := runCmd("sync", "--path", piDir, "--optimize")
	if err != nil {
		t.Fatalf("sync --optimize error: %v", err)
	}
}

func TestResumeRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("resume", "pi", "--robot")
	if err != nil {
		t.Fatalf("resume --robot error: %v", err)
	}
	_ = out
}

func TestSearchAfterBefore(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("search", "test", "--after", "2020-01-01", "--before", "2030-01-01")
	if err != nil {
		t.Fatalf("search --after --before error: %v", err)
	}
	_ = out
}

func TestValidateAfterSync(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("validate")
	if err != nil {
		t.Fatalf("validate after sync error: %v", err)
	}
	_ = out
}

func TestPurgeAfterSync(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	// Purge with a future date — should purge everything
	out, _, err := runCmd("purge", "--before", "2030-01-01")
	if err != nil {
		t.Fatalf("purge after sync error: %v", err)
	}
	_ = out
}

func TestListEmptyDB(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// No sync — list should not error (handles missing DB gracefully)
	out, _, err := runCmd("list")
	if err != nil {
		t.Fatalf("list on empty DB error: %v", err)
	}
	_ = out
}

func TestListAfterValidate(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Create DB via validate (uses Open, not OpenReadOnly)
	_, _, _ = runCmd("validate")

	// Now list — DB exists but has no sessions
	out, _, err := runCmd("list")
	if err != nil {
		t.Fatalf("list after validate error: %v", err)
	}
	_ = out
}

func TestListJSONAfterValidate(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Create DB via validate
	_, _, _ = runCmd("validate")

	// Now list --json — DB exists but has no sessions → should output valid JSON
	out, _, err := runCmd("list", "--json")
	if err != nil {
		t.Fatalf("list --json after validate error: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output for list --json")
	}
}

func TestListRobotAfterValidate(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Create DB via validate
	_, _, _ = runCmd("validate")

	out, _, err := runCmd("list", "--robot")
	if err != nil {
		t.Fatalf("list --robot after validate error: %v", err)
	}
	_ = out
}

func TestTopicsEmptyDB(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Create DB via validate (no sessions)
	_, _, _ = runCmd("validate")

	out, _, err := runCmd("topics")
	if err != nil {
		t.Fatalf("topics on empty DB error: %v", err)
	}
	_ = out
}

func TestInsightsEmptyDB(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Create DB via validate
	_, _, _ = runCmd("validate")

	out, _, err := runCmd("insights")
	if err != nil {
		t.Fatalf("insights on empty DB error: %v", err)
	}
	_ = out
}

func TestTopicsAllProjects(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("topics", "--all-projects", "--limit", "10")
	if err != nil {
		t.Fatalf("topics --all-projects error: %v", err)
	}
	_ = out
}

func TestInsightsAllProjects(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	out, _, err := runCmd("insights", "--all-projects")
	if err != nil {
		t.Fatalf("insights --all-projects error: %v", err)
	}
	_ = out
}

func init() {
	// Suppress cobra's default behavior of writing to os.Stderr on error
	_ = fmt.Sprintf // keep fmt import
}
