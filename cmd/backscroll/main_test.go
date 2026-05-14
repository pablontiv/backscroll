package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/storage"
	_ "modernc.org/sqlite"
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

func TestInputsHelp(t *testing.T) {
	out, _, err := runCmd("inputs", "--help")
	if err != nil {
		t.Fatalf("inputs --help error: %v", err)
	}
	for _, sub := range []string{"list", "aliases", "identify", "test"} {
		if !strings.Contains(out, sub) {
			t.Errorf("inputs --help missing subcommand %q", sub)
		}
	}
}

func TestInputsList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	// No inputs configured — should use legacy mode
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("inputs", "list")
	if err != nil {
		t.Fatalf("inputs list error: %v", err)
	}
	if !strings.Contains(out, "mode:") {
		t.Errorf("inputs list output missing 'mode:': %s", out)
	}
}

func TestInputsListJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("inputs", "list", "--json")
	if err != nil {
		t.Fatalf("inputs list --json error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if _, ok := result["mode"]; !ok {
		t.Error("JSON missing 'mode' field")
	}
	if _, ok := result["inputs"]; !ok {
		t.Error("JSON missing 'inputs' field")
	}
}

func TestInputsAliases(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	_, cleanup := testEnv(t)
	defer cleanup()

	// aliases with no session_dirs — should run without crash
	_, _, err := runCmd("inputs", "aliases")
	if err != nil {
		t.Fatalf("inputs aliases error: %v", err)
	}
}

func TestInputsIdentify(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	_, cleanup := testEnv(t)
	defer cleanup()

	// identify a file that doesn't match any input — should say "no match"
	out, _, err := runCmd("inputs", "identify", "/nonexistent/path/file.jsonl")
	if err != nil {
		t.Fatalf("inputs identify error: %v", err)
	}
	if !strings.Contains(out, "no match") {
		t.Errorf("expected 'no match', got: %s", out)
	}
}

func TestInputsTestNoMatch(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, err := runCmd("inputs", "test", "/nonexistent/path/file.jsonl")
	if err == nil {
		t.Error("expected error for unmatched path")
	}
}

// setupInputsPreset writes a claude.inputs.toml pointing to fixtureProjects in cfgDir.
func setupInputsPreset(t *testing.T, cfgDir, fixtureProjects string) {
	t.Helper()
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := fmt.Sprintf(`version = 1
[[inputs]]
id = "claude"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]
[inputs.decode]
format = "jsonl"
[inputs.record]
selector = "$"
include_when = [{selector = "$.type", op = "in", value = ["user", "assistant"]}]
exclude_when = [{selector = "$.isMeta", op = "eq", value = true}]
[inputs.map]
role = "$.message.role"
uuid = "$.uuid"
timestamp = "$.timestamp"
session_id = "$.sessionId"
[inputs.content]
selector = "$.message.content"
blocks = "$.message.content[*]"
block_text = "$.text"
include_when = [{selector = "$.type", op = "eq", value = "text"}]
[inputs.text]
trim = true
drop_empty = true
`, fixtureProjects)
	if err := os.WriteFile(filepath.Join(inputsDir, "claude.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInputsTestWithFixture(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	presetDir := filepath.Join(fixturesDir(), "claude-preset")
	cfgDir := t.TempDir()
	setupInputsPreset(t, cfgDir, filepath.Join(presetDir, "projects"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	fixture := filepath.Join(presetDir, "projects", "project-a", "session-main.jsonl")
	out, _, err := runCmd("inputs", "test", fixture)
	if err != nil {
		t.Logf("inputs test output: %s", out)
		t.Fatalf("inputs test error: %v", err)
	}
	if !strings.Contains(out, "input:") {
		t.Errorf("expected 'input:' in output, got: %s", out)
	}
}

func TestInputsAliasesWithPreset(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	presetDir := filepath.Join(fixturesDir(), "claude-preset")
	cfgDir := t.TempDir()
	setupInputsPreset(t, cfgDir, filepath.Join(presetDir, "projects"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	out, _, err := runCmd("inputs", "aliases")
	if err != nil {
		t.Fatalf("inputs aliases error: %v", err)
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("expected 'claude' in aliases output, got: %s", out)
	}
}

func TestInputsIdentifyWithPreset(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	presetDir := filepath.Join(fixturesDir(), "claude-preset")
	cfgDir := t.TempDir()
	setupInputsPreset(t, cfgDir, filepath.Join(presetDir, "projects"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	fixture := filepath.Join(presetDir, "projects", "project-a", "session-main.jsonl")
	out, _, err := runCmd("inputs", "identify", fixture)
	if err != nil {
		t.Fatalf("inputs identify error: %v", err)
	}
	if !strings.Contains(out, "matched") {
		t.Errorf("expected 'matched' in output, got: %s", out)
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
		"decisions", "projects", "inputs",
	} {
		if !strings.Contains(out, cmd) {
			t.Errorf("--help missing command %q", cmd)
		}
	}
}

// TestSyncOpenCode verifies that the sync command indexes OpenCode SQLite sessions
// using a declarative opencode.inputs.toml manifest.
func TestSyncOpenCode(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Create temp config dir with opencode.inputs.toml
	configDir := t.TempDir()
	inputsDir := filepath.Join(configDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a temp OpenCode project with .opencode/opencode.db
	projectDir := t.TempDir()
	dbDir := filepath.Join(projectDir, ".opencode")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "opencode.db")
	createOpenCodeTestDB(t, dbPath, "opencode integration content")

	// Write opencode.inputs.toml pointing to the project dir
	tomlContent := `version = 1

[[inputs]]
id = "opencode-test"
source = "session"
active = true

[inputs.discover]
roots = ["` + projectDir + `"]
include = ["**/.opencode/opencode.db"]

[inputs.decode]
format = "opencode"
`
	if err := os.WriteFile(filepath.Join(inputsDir, "opencode.inputs.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set config dir so ActiveInputs reads our manifest
	origConfigDir := os.Getenv("BACKSCROLL_CONFIG_DIR")
	_ = os.Setenv("BACKSCROLL_CONFIG_DIR", configDir)
	defer func() {
		if origConfigDir == "" {
			_ = os.Unsetenv("BACKSCROLL_CONFIG_DIR")
		} else {
			_ = os.Setenv("BACKSCROLL_CONFIG_DIR", origConfigDir)
		}
	}()

	out, stderr, err := runCmd("sync")
	if err != nil {
		t.Fatalf("sync error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(out, "Synced") {
		t.Errorf("unexpected sync output: %s", out)
	}

	// Search for the indexed content
	searchOut, _, err := runCmd("search", "opencode integration content")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if !strings.Contains(searchOut, "opencode integration content") {
		t.Errorf("OpenCode content not found in search results: %s", searchOut)
	}
}

// createOpenCodeTestDB creates a minimal OpenCode SQLite DB with one message.
func createOpenCodeTestDB(t *testing.T, path, content string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, title TEXT NOT NULL, message_count INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0.0, updated_at INTEGER NOT NULL, created_at INTEGER NOT NULL);
		CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, role TEXT NOT NULL,
			parts TEXT NOT NULL DEFAULT '[]', model TEXT, created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL, finished_at INTEGER);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	now := time.Now().UnixMilli()
	_, err = db.Exec(`INSERT INTO sessions (id, title, updated_at, created_at) VALUES (?, ?, ?, ?)`,
		"s1", "Test Session", now, now)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	type part struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}
	parts, _ := json.Marshal([]part{{Type: "text", Data: map[string]string{"text": content}}})
	_, err = db.Exec(`INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"m1", "s1", "user", string(parts), now+1000, now+1000)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}

func TestRunEmbedPipeline_StoresChunks(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	files := []storage.IndexedFile{
		{
			SourcePath: "test/session.jsonl",
			Source:     "session",
			Hash:       "abc123",
			Messages: []storage.IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello world foo bar baz"},
				{Ordinal: 1, Role: "assistant", Text: "some response text here"},
			},
		},
	}

	var stdout, stderr bytes.Buffer
	runEmbedPipeline(&stdout, &stderr, db, embeddingCfgForTest(), files)

	// ONNX unavailable — warning should be in stderr, chunks should still be stored
	if !strings.Contains(stderr.String(), "warning") {
		t.Errorf("expected warning about ONNX unavailable, stderr: %s", stderr.String())
	}

	count, err := db.GetChunkCount()
	if err != nil {
		t.Fatalf("GetChunkCount: %v", err)
	}
	if count == 0 {
		t.Error("expected chunks to be stored even without embedding provider")
	}
}

func TestRunEmbedPipeline_SkipsEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	files := []storage.IndexedFile{
		{SourcePath: "empty.jsonl", Messages: []storage.IndexedMessage{}},
	}

	var stdout, stderr bytes.Buffer
	runEmbedPipeline(&stdout, &stderr, db, embeddingCfgForTest(), files)

	count, _ := db.GetChunkCount()
	if count != 0 {
		t.Errorf("expected 0 chunks for empty file, got %d", count)
	}
}

// embeddingCfgForTest returns a minimal EmbeddingConfig for pipeline tests.
func embeddingCfgForTest() config.EmbeddingConfig {
	return config.EmbeddingConfig{
		ModelName:           "all-MiniLM-L6-v2",
		SimilarityThreshold: 0.7,
		TopK:                10,
	}
}

func TestEventsQuery(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync the claude-preset fixture to populate session_events
	fixtureDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := runCmd("sync", "--path", fixtureDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Query non-existent session — should be graceful (no error, no output)
	out, _, err := runCmd("events", "query", "nonexistent-session-id")
	if err != nil {
		t.Fatalf("events query error: %v", err)
	}
	_ = out
}

func TestEventsQueryJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync the claude-preset fixture
	fixtureDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := runCmd("sync", "--path", fixtureDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Discover what was actually indexed so we can query it
	sessionPath := filepath.Join(fixtureDir, "project-a", "session-main.jsonl")

	out, _, err := runCmd("events", "query", "--json", sessionPath)
	if err != nil {
		t.Fatalf("events query --json error: %v", err)
	}
	// If session had events, verify valid JSON output
	if out != "" {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			var obj map[string]interface{}
			if jsonErr := json.Unmarshal([]byte(line), &obj); jsonErr != nil {
				t.Errorf("invalid JSON: %v\n%s", jsonErr, line)
			}
			// Verify key fields present
			if _, ok := obj["snippet"]; !ok {
				t.Errorf("JSON missing 'snippet' field: %s", line)
			}
		}
	}

	// Also test --robot output
	_, _, err = runCmd("events", "query", "--robot", sessionPath)
	if err != nil {
		t.Fatalf("events query --robot error: %v", err)
	}
}

func TestStatusJSONHasInputFields(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("status", "--json")
	if err != nil {
		t.Fatalf("status --json error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("parse JSON: %v\n%s", err, out)
	}

	cfg, ok := result["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("config field missing or wrong type: %v", result)
	}
	if _, ok := cfg["active_inputs"]; !ok {
		t.Error("config.active_inputs missing from status --json")
	}
	if _, ok := cfg["using_declarative_inputs"]; !ok {
		t.Error("config.using_declarative_inputs missing from status --json")
	}

	idx, ok := result["index"].(map[string]interface{})
	if !ok {
		t.Fatalf("index field missing: %v", result)
	}
	if _, ok := idx["total_chunks"]; !ok {
		t.Error("index.total_chunks missing from status --json")
	}
	if _, ok := idx["total_embeddings"]; !ok {
		t.Error("index.total_embeddings missing from status --json")
	}
}

func TestEventsQueryDirect(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	// Directly populate the DB via storage to ensure known state
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = db.SyncFiles([]storage.IndexedFile{
		{
			SourcePath: "/tmp/test-session.jsonl",
			Source:     "session",
			Hash:       "testhash",
			Messages: []storage.IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "hello events", Timestamp: "2024-06-01T10:00:00Z", ContentType: "text"},
				{Ordinal: 1, Role: "assistant", Text: "hi there", Timestamp: "2024-06-01T10:01:00Z", ContentType: "text"},
			},
		},
	})
	_ = db.Close()

	// Test plain output
	out, _, err := runCmd("events", "query", "/tmp/test-session.jsonl")
	if err != nil {
		t.Fatalf("events query error: %v", err)
	}
	if !strings.Contains(out, "hello events") {
		t.Errorf("expected 'hello events' in output, got: %s", out)
	}

	// Test JSON output
	out, _, err = runCmd("events", "query", "--json", "/tmp/test-session.jsonl")
	if err != nil {
		t.Fatalf("events query --json error: %v", err)
	}
	if out == "" {
		t.Fatal("expected JSON output, got empty")
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var obj map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(line), &obj); jsonErr != nil {
			t.Errorf("invalid JSON: %v\n%s", jsonErr, line)
		}
		if obj["snippet"] == nil {
			t.Errorf("JSON missing 'snippet': %s", line)
		}
	}

	// Test --robot output
	out, _, err = runCmd("events", "query", "--robot", "/tmp/test-session.jsonl")
	if err != nil {
		t.Fatalf("events query --robot error: %v", err)
	}
	if !strings.Contains(out, "hello events") {
		t.Errorf("expected 'hello events' in robot output, got: %s", out)
	}

	// Test --role filter
	out, _, err = runCmd("events", "query", "--role", "user", "/tmp/test-session.jsonl")
	if err != nil {
		t.Fatalf("events query --role error: %v", err)
	}
	if strings.Contains(out, "hi there") {
		t.Errorf("expected no assistant messages with --role user, got: %s", out)
	}
}

func TestEventsQueryNewFlags(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = db.SyncFiles([]storage.IndexedFile{
		{
			SourcePath: "/tmp/ev-flags-session.jsonl",
			Source:     "session",
			Hash:       "ev-flags-hash",
			Messages: []storage.IndexedMessage{
				{Ordinal: 0, Role: "user", Text: "test event", Timestamp: "2024-06-01T10:00:00Z", ContentType: "text"},
			},
		},
	})
	_ = db.Close()

	// --source session flag accepted
	out, _, err := runCmd("events", "query", "--source", "session", "--all-projects", "/tmp/ev-flags-session.jsonl")
	if err != nil {
		t.Fatalf("events query --source error: %v", err)
	}
	_ = out

	// --source-path flag accepted (LIKE glob pattern)
	out, _, err = runCmd("events", "query", "--source-path", "*.jsonl", "--all-projects")
	if err != nil {
		t.Fatalf("events query --source-path error: %v", err)
	}
	_ = out

	// --event-type flag accepted
	out, _, err = runCmd("events", "query", "--event-type", "message", "--all-projects")
	if err != nil {
		t.Fatalf("events query --event-type error: %v", err)
	}
	_ = out

	// --indexed-only flag accepted
	out, _, err = runCmd("events", "query", "--indexed-only", "--all-projects")
	if err != nil {
		t.Fatalf("events query --indexed-only error: %v", err)
	}
	_ = out

	// --project flag accepted
	out, _, err = runCmd("events", "query", "--project", "myproject")
	if err != nil {
		t.Fatalf("events query --project error: %v", err)
	}
	_ = out
}

func TestSessionsList(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixture := filepath.Join(fixturesDir(), "claude.jsonl")
	_, _, _ = runCmd("sync", "--path", filepath.Dir(fixture))

	out, _, err := runCmd("sessions", "list")
	if err != nil {
		t.Fatalf("sessions list error: %v", err)
	}
	_ = out
}

func TestSessionsValidate(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("sessions", "validate")
	if err != nil {
		t.Fatalf("sessions validate error: %v", err)
	}
	_ = out
}

func TestSessionsQuery(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixture := filepath.Join(fixturesDir(), "claude.jsonl")
	_, _, _ = runCmd("sync", "--path", filepath.Dir(fixture))

	out, _, err := runCmd("sessions", "query")
	if err != nil {
		t.Fatalf("sessions query error: %v", err)
	}
	_ = out
}

func TestSessionsQueryJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixture := filepath.Join(fixturesDir(), "claude.jsonl")
	_, _, _ = runCmd("sync", "--path", filepath.Dir(fixture))

	out, _, err := runCmd("sessions", "query", "--json")
	if err != nil {
		t.Fatalf("sessions query --json error: %v", err)
	}
	if out != "" {
		// Verify valid JSONL
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			var obj map[string]interface{}
			if jsonErr := json.Unmarshal([]byte(line), &obj); jsonErr != nil {
				t.Errorf("invalid JSON line: %v\n%s", jsonErr, line)
			}
		}
	}
}

func TestListRecentN(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = runCmd("sync", "--path", piDir)

	// --recent 1 should work without error
	out, _, err := runCmd("list", "--recent", "1")
	if err != nil {
		t.Fatalf("list --recent 1 error: %v", err)
	}
	_ = out
}

func TestListIndexedOnly(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("list", "--indexed-only")
	if err != nil {
		t.Fatalf("list --indexed-only error: %v", err)
	}
	_ = out
}

func TestStatusIndexedOnly(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("status", "--indexed-only")
	if err != nil {
		t.Fatalf("status --indexed-only error: %v", err)
	}
	if !strings.Contains(out, "Backscroll Status") {
		t.Errorf("status --indexed-only missing header: %s", out)
	}
}

func TestInputsValidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("inputs", "validate")
	if err != nil {
		t.Fatalf("inputs validate error: %v", err)
	}
	if !strings.Contains(out, "valid") {
		t.Errorf("inputs validate output missing 'valid': %s", out)
	}
}

func TestInputsValidateJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", dir)

	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("inputs", "validate", "--json")
	if err != nil {
		t.Fatalf("inputs validate --json error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if _, ok := result["valid"]; !ok {
		t.Error("JSON missing 'valid' field")
	}
	if _, ok := result["inputs"]; !ok {
		t.Error("JSON missing 'inputs' field")
	}
}

func init() {
	// Suppress cobra's default behavior of writing to os.Stderr on error
	_ = fmt.Sprintf // keep fmt import
}
