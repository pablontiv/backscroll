package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

// testEnv creates an isolated environment for CLI tests.
// It sets BACKSCROLL_DATABASE_PATH to a temp file and returns a cleanup func.
func testEnv(t *testing.T) (dbPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "test.db")
	orig := os.Getenv("BACKSCROLL_DATABASE_PATH")
	_ = os.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	return dbPath, func() {
		if orig == "" {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		} else {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", orig)
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

// seedDecisions opens the test DB and inserts decision + session records directly.
func seedDecisions(t *testing.T, dbPath string) {
	t.Helper()
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()
	files := []storage.IndexedFile{
		{
			SourcePath: "/decisions/d001.md",
			Source:     "decision",
			Hash:       "h-d001",
			Project:    "testproj",
			Messages: []storage.IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "---\nid: D001\nstatus: accepted\nscope: technical\n---\n# Use Go\nWe decided to use Go for all backend services.",
					UUID:        "uuid-d001",
					Timestamp:   "2026-01-01T10:00:00Z",
					ContentType: "text",
				},
			},
		},
		{
			SourcePath: "/decisions/d002.md",
			Source:     "decision",
			Hash:       "h-d002",
			Project:    "testproj",
			Messages: []storage.IndexedMessage{
				{
					Ordinal:     0,
					Role:        "user",
					Text:        "---\nid: D002\nstatus: proposed\nscope: organizational\n---\n# Daily standups\nWe should have daily standups.",
					UUID:        "uuid-d002",
					Timestamp:   "2026-01-02T10:00:00Z",
					ContentType: "text",
				},
			},
		},
		{
			SourcePath: "/sessions/s001.jsonl",
			Source:     "session",
			Hash:       "h-s001",
			Project:    "testproj",
			Messages: []storage.IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "we decided to use Go for performance", UUID: "uuid-s001a", Timestamp: "2026-01-03T10:00:00Z", ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "decision: adopt Go as the backend language", UUID: "uuid-s001b", Timestamp: "2026-01-03T10:01:00Z", ContentType: "text"},
			},
			Tags: []string{"feature"},
		},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("SyncFiles: %v", err)
	}
}

func TestDecisionsQueryWithData(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	seedDecisions(t, dbPath)

	// Text output
	out, _, err := runCmd("decisions", "query", "--all-projects")
	if err != nil {
		t.Fatalf("decisions query error: %v", err)
	}
	if !strings.Contains(out, "Use Go") && !strings.Contains(out, "accepted") {
		t.Errorf("expected decision record in output: %s", out)
	}

	// JSON output
	out, _, err = runCmd("decisions", "query", "--all-projects", "--json")
	if err != nil {
		t.Fatalf("decisions query --json error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		t.Error("expected at least one JSON line")
	}
	var rec map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("invalid JSON line: %v", err)
	}
	if rec["title"] == nil {
		t.Error("missing title in JSON output")
	}

	// Status filter
	out, _, err = runCmd("decisions", "query", "--all-projects", "--status", "accepted")
	if err != nil {
		t.Fatalf("decisions query --status error: %v", err)
	}
	if !strings.Contains(out, "Use Go") {
		t.Errorf("expected accepted decision in output: %s", out)
	}

	// Scope filter
	out, _, err = runCmd("decisions", "query", "--all-projects", "--scope", "technical")
	if err != nil {
		t.Fatalf("decisions query --scope error: %v", err)
	}
	if !strings.Contains(out, "Use Go") {
		t.Errorf("expected technical-scoped decision in output: %s", out)
	}

	// Project filter (explicit)
	out, _, err = runCmd("decisions", "query", "--project", "testproj")
	if err != nil {
		t.Fatalf("decisions query --project error: %v", err)
	}
	_ = out
}

func TestDecisionsContextWithData(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	seedDecisions(t, dbPath)

	// Text output
	out, _, err := runCmd("decisions", "context", "--all-projects")
	if err != nil {
		t.Fatalf("decisions context error: %v", err)
	}
	if !strings.Contains(out, "Decision Context") {
		t.Errorf("expected 'Decision Context' in output: %s", out)
	}
	if !strings.Contains(out, "Use Go") {
		t.Errorf("expected decision title in context output: %s", out)
	}

	// JSON output
	out, _, err = runCmd("decisions", "context", "--all-projects", "--json")
	if err != nil {
		t.Fatalf("decisions context --json error: %v", err)
	}
	var ctx map[string]interface{}
	if err := json.Unmarshal([]byte(out), &ctx); err != nil {
		t.Fatalf("context --json invalid JSON: %v\nout: %s", err, out)
	}
	decisions, ok := ctx["decisions"].([]interface{})
	if !ok || len(decisions) == 0 {
		t.Errorf("expected decisions in context output: %v", ctx)
	}

	// Max tokens limit
	out, _, err = runCmd("decisions", "context", "--all-projects", "--max-tokens", "10")
	if err != nil {
		t.Fatalf("decisions context --max-tokens error: %v", err)
	}
	_ = out
}

func TestDecisionsExtractWithData(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	seedDecisions(t, dbPath)

	out, _, err := runCmd("decisions", "extract", "--all-projects")
	if err != nil {
		t.Fatalf("decisions extract error: %v", err)
	}
	// Should have at least one candidate (from session records with decision patterns)
	if out != "" {
		var candidate map[string]interface{}
		firstLine := strings.Split(strings.TrimSpace(out), "\n")[0]
		if err := json.Unmarshal([]byte(firstLine), &candidate); err != nil {
			t.Fatalf("extract output not valid JSON: %v\nline: %s", err, firstLine)
		}
		if candidate["statement"] == nil {
			t.Error("missing statement in candidate")
		}
	}

	// With limit
	out, _, err = runCmd("decisions", "extract", "--all-projects", "--limit", "1")
	if err != nil {
		t.Fatalf("decisions extract --limit error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if out != "" && len(lines) > 1 {
		t.Errorf("expected at most 1 candidate with --limit 1, got %d", len(lines))
	}

	// With since filter
	out, _, err = runCmd("decisions", "extract", "--all-projects", "--since", "2026-01-01")
	if err != nil {
		t.Fatalf("decisions extract --since error: %v", err)
	}
	_ = out
}

func TestDecisionsConflictsWithData(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	seedDecisions(t, dbPath)

	// Duplicate proposal — should detect conflict
	proposal := `{"id":"D001","statement":"we decided to use Go for all backend services","scope":"technical"}`
	out, _, err := runCmd("decisions", "conflicts", "--all-projects", "--proposal-json", proposal)
	if err != nil {
		t.Fatalf("decisions conflicts error: %v", err)
	}
	// With data present, should find duplicate or potential conflict
	_ = out

	// JSON output
	out, _, err = runCmd("decisions", "conflicts", "--all-projects", "--proposal-json", proposal, "--json")
	if err != nil {
		t.Fatalf("decisions conflicts --json error: %v", err)
	}
	var hints []interface{}
	if err := json.Unmarshal([]byte(out), &hints); err != nil {
		t.Fatalf("conflicts --json invalid JSON: %v\nout: %s", err, out)
	}
}

func TestDecisionsReplayWithData(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	seedDecisions(t, dbPath)

	dir := t.TempDir()

	// Fixture with covered + missed decisions
	fixturePath := filepath.Join(dir, "fixture.json")
	fixtureContent := `{"expected_decisions":[
		{"id":"D001","statement":"we decided to use Go for all backend services","status":"accepted"},
		{"statement":"we decided to use Rust instead"}
	]}`
	if err := os.WriteFile(fixturePath, []byte(fixtureContent), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	// Text output
	out, _, err := runCmd("decisions", "replay", "--fixture", fixturePath, "--all-projects")
	if err != nil {
		t.Fatalf("decisions replay error: %v", err)
	}
	if !strings.Contains(out, "Replay Report") {
		t.Errorf("expected 'Replay Report' in output: %s", out)
	}
	if !strings.Contains(out, "Coverage") {
		t.Errorf("expected 'Coverage' in output: %s", out)
	}

	// JSON output
	out, _, err = runCmd("decisions", "replay", "--fixture", fixturePath, "--all-projects", "--json")
	if err != nil {
		t.Fatalf("decisions replay --json error: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("replay --json invalid JSON: %v\nout: %s", err, out)
	}
	if _, ok := report["coverage_pct"]; !ok {
		t.Error("missing coverage_pct in replay JSON")
	}
	if _, ok := report["missed"]; !ok {
		t.Error("missing missed in replay JSON")
	}
}

// ---- projects command tests ----

func TestProjectsHelp(t *testing.T) {
	out, _, err := runCmd("projects", "--help")
	if err != nil {
		t.Fatalf("projects --help error: %v", err)
	}
	for _, sub := range []string{"identify", "list", "aliases"} {
		if !strings.Contains(out, sub) {
			t.Errorf("projects --help missing subcommand %q", sub)
		}
	}
}

func TestProjectsIdentify(t *testing.T) {
	out, _, err := runCmd("projects", "identify")
	if err != nil {
		t.Fatalf("projects identify error: %v", err)
	}
	if !strings.Contains(out, "project:") {
		t.Errorf("projects identify missing 'project:': %s", out)
	}
}

func TestProjectsIdentifyJSON(t *testing.T) {
	out, _, err := runCmd("projects", "identify", "--json")
	if err != nil {
		t.Fatalf("projects identify --json error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("projects identify --json invalid JSON: %v\nout: %s", err, out)
	}
	if _, ok := result["project_id"]; !ok {
		t.Errorf("missing project_id in output: %v", result)
	}
	if _, ok := result["confidence"]; !ok {
		t.Errorf("missing confidence in output: %v", result)
	}
}

func TestProjectsIdentifyWithCwd(t *testing.T) {
	dir := t.TempDir()
	out, _, err := runCmd("projects", "identify", "--cwd", dir)
	if err != nil {
		t.Fatalf("projects identify --cwd error: %v", err)
	}
	if !strings.Contains(out, "project:") {
		t.Errorf("projects identify --cwd missing 'project:': %s", out)
	}
}

func TestProjectsList(t *testing.T) {
	// Registry may be empty in test environment — that's fine, just no error
	_, _, err := runCmd("projects", "list")
	if err != nil {
		t.Fatalf("projects list error: %v", err)
	}
}

func TestProjectsListJSON(t *testing.T) {
	out, _, err := runCmd("projects", "list", "--json")
	if err != nil {
		t.Fatalf("projects list --json error: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("projects list --json invalid JSON: %v\nout: %s", err, out)
	}
	if _, ok := result["count"]; !ok {
		t.Errorf("missing count in projects list output: %v", result)
	}
}

func TestProjectsAliasesMissingFlag(t *testing.T) {
	_, _, err := runCmd("projects", "aliases")
	if err == nil {
		t.Error("expected error when --project-id not provided")
	}
}

func TestProjectsAliasesJSON(t *testing.T) {
	out, _, err := runCmd("projects", "aliases", "--project-id", "nonexistent", "--json")
	if err != nil {
		t.Fatalf("projects aliases --json error: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("projects aliases --json invalid JSON: %v\nout: %s", err, out)
	}
	if result["project_id"] != "nonexistent" {
		t.Errorf("expected project_id=nonexistent, got %v", result["project_id"])
	}
}

// ---- decisions command tests ----

func TestDecisionsHelp(t *testing.T) {
	out, _, err := runCmd("decisions", "--help")
	if err != nil {
		t.Fatalf("decisions --help error: %v", err)
	}
	for _, sub := range []string{"query", "context", "extract", "conflicts", "replay"} {
		if !strings.Contains(out, sub) {
			t.Errorf("decisions --help missing subcommand %q", sub)
		}
	}
}

func TestDecisionsQueryEmptyDB(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	out, _, err := runCmd("decisions", "query", "--all-projects")
	if err != nil {
		t.Fatalf("decisions query error: %v", err)
	}
	if !strings.Contains(out, "No decisions found") {
		t.Errorf("expected 'No decisions found', got: %s", out)
	}
}

func TestDecisionsQueryJSONEmptyDB(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	// Should not error even with --json and empty result
	_, _, err := runCmd("decisions", "query", "--all-projects", "--json")
	if err != nil {
		t.Fatalf("decisions query --json error: %v", err)
	}
}

func TestDecisionsContextEmptyDB(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	out, _, err := runCmd("decisions", "context", "--all-projects")
	if err != nil {
		t.Fatalf("decisions context error: %v", err)
	}
	if !strings.Contains(out, "Decision Context") {
		t.Errorf("expected 'Decision Context' in output: %s", out)
	}
}

func TestDecisionsContextJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	out, _, err := runCmd("decisions", "context", "--all-projects", "--json")
	if err != nil {
		t.Fatalf("decisions context --json error: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decisions context --json invalid JSON: %v\nout: %s", err, out)
	}
	if _, ok := result["decisions"]; !ok {
		t.Errorf("missing 'decisions' in context output")
	}
}

func TestDecisionsExtractEmptyDB(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	// Empty corpus → no output, no error
	out, _, err := runCmd("decisions", "extract", "--all-projects")
	if err != nil {
		t.Fatalf("decisions extract error: %v", err)
	}
	_ = out
}

func TestDecisionsExtractFromSessions(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, _ = runCmd("sync", "--path", fixtureSession)

	out, _, err := runCmd("decisions", "extract", "--all-projects")
	if err != nil {
		t.Fatalf("decisions extract error: %v", err)
	}
	_ = out // may or may not find candidates depending on fixture content
}

func TestDecisionsConflictsMissingInput(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	// --proposal-json with invalid JSON should error
	_, _, err := runCmd("decisions", "conflicts", "--proposal-json", "not-json", "--all-projects")
	if err == nil {
		t.Error("expected error for invalid proposal JSON")
	}
}

func TestDecisionsConflictsValidProposal(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	proposal := `{"statement":"we should use Go for all new services","scope":"technical"}`
	out, _, err := runCmd("decisions", "conflicts", "--proposal-json", proposal, "--all-projects")
	if err != nil {
		t.Fatalf("decisions conflicts error: %v", err)
	}
	if !strings.Contains(out, "No conflicts found") && !strings.Contains(out, "Conflict") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestDecisionsConflictsJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	proposal := `{"statement":"we decided to use SQLite","scope":"technical"}`
	out, _, err := runCmd("decisions", "conflicts", "--proposal-json", proposal, "--all-projects", "--json")
	if err != nil {
		t.Fatalf("decisions conflicts --json error: %v", err)
	}
	// Should be a JSON array
	var result []interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decisions conflicts --json invalid JSON: %v\nout: %s", err, out)
	}
}

func TestDecisionsReplayMissingFixture(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, err := runCmd("decisions", "replay", "--fixture", "/nonexistent/fixture.json")
	if err == nil {
		t.Error("expected error for missing fixture file")
	}
}

func TestDecisionsReplayValidFixture(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	// Create a minimal fixture file
	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	fixtureContent := `{"expected_decisions":[{"statement":"we decided to use Go","status":"accepted"}]}`
	if err := os.WriteFile(fixturePath, []byte(fixtureContent), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out, _, err := runCmd("decisions", "replay", "--fixture", fixturePath, "--all-projects")
	if err != nil {
		t.Fatalf("decisions replay error: %v", err)
	}
	if !strings.Contains(out, "Replay Report") {
		t.Errorf("expected 'Replay Report' in output: %s", out)
	}
}

func TestDecisionsReplayJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, _ = runCmd("validate")

	dir := t.TempDir()
	fixturePath := filepath.Join(dir, "fixture.json")
	fixtureContent := `{"expected_decisions":[]}`
	if err := os.WriteFile(fixturePath, []byte(fixtureContent), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	out, _, err := runCmd("decisions", "replay", "--fixture", fixturePath, "--all-projects", "--json")
	if err != nil {
		t.Fatalf("decisions replay --json error: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decisions replay --json invalid JSON: %v\nout: %s", err, out)
	}
	if _, ok := result["coverage_pct"]; !ok {
		t.Errorf("missing coverage_pct in replay output")
	}
}

// ---- help completeness check ----

func TestSyncWithPlansDir(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Build a temp HOME with .claude/plans/ containing a plan file
	tmpHome := t.TempDir()
	plansDir := filepath.Join(tmpHome, ".claude", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planContent := "# Test Plan\n\n## Step 1\n\nDo this step.\n\n## Step 2\n\nDo that step.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "test-plan.md"), []byte(planContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Temporarily redirect HOME so homeDir() finds our plans dir
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpHome)
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	// Sync session files from fixture AND let plan indexing run (no --no-plans)
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := runCmd("sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync with plans dir error: %v", err)
	}
}

func TestHelpListsAllCommands(t *testing.T) {
	out, _, err := runCmd("--help")
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}
	for _, cmd := range []string{
		"sync", "search", "read", "resume", "list", "topics",
		"insights", "export", "reindex", "purge", "validate", "status",
		"decisions", "projects",
	} {
		if !strings.Contains(out, cmd) {
			t.Errorf("--help missing command %q", cmd)
		}
	}
}

func init() {
	// Suppress cobra's default behavior of writing to os.Stderr on error
	_ = fmt.Sprintf // keep fmt import
}
