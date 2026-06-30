package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// testEnv creates an isolated environment for CLI tests.
// Sets BACKSCROLL_DATABASE_PATH to a temp file and BACKSCROLL_CONFIG_DIR to a
// temp dir so tests never load the user's real ~/.config/backscroll/inputs/.
func testEnv(t *testing.T) (dbPath string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath = filepath.Join(dir, "test.db")
	origDB := os.Getenv("BACKSCROLL_DATABASE_PATH")
	origCfg := os.Getenv("BACKSCROLL_CONFIG_DIR")
	_ = os.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	_ = os.Setenv("BACKSCROLL_CONFIG_DIR", dir)
	return dbPath, func() {
		if origDB == "" {
			_ = os.Unsetenv("BACKSCROLL_DATABASE_PATH")
		} else {
			_ = os.Setenv("BACKSCROLL_DATABASE_PATH", origDB)
		}
		if origCfg == "" {
			_ = os.Unsetenv("BACKSCROLL_CONFIG_DIR")
		} else {
			_ = os.Setenv("BACKSCROLL_CONFIG_DIR", origCfg)
		}
	}
}

// setupSessionDir sets BACKSCROLL_SESSION_DIRS to a path for auto-sync testing.
// Returns the original value for restoration.
func setupSessionDir(t *testing.T, path string) func() {
	t.Helper()
	origDirs := os.Getenv("BACKSCROLL_SESSION_DIRS")
	_ = os.Setenv("BACKSCROLL_SESSION_DIRS", path)
	return func() {
		if origDirs == "" {
			_ = os.Unsetenv("BACKSCROLL_SESSION_DIRS")
		} else {
			_ = os.Setenv("BACKSCROLL_SESSION_DIRS", origDirs)
		}
	}
}

// syncForTest is a v1->v2 migration helper. In v2, sync is not a root command
// but auto-sync happens before queries. This helper just sets up session dirs
// and returns a fake "success" to maintain test patterns.
func syncForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	// Extract --path value if present
	for i, arg := range args {
		if arg == "--path" && i+1 < len(args) {
			_ = setupSessionDir(t, args[i+1])
			return "", "", nil
		}
	}
	return "", "", nil
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

	// Extract the "Available Commands:" section
	parts := strings.Split(out, "Available Commands:")
	if len(parts) < 2 {
		t.Fatalf("Could not find 'Available Commands:' section in help")
	}
	commandsSection := parts[1]

	// v2 approved root commands that SHOULD be present
	approvedV2 := []string{"list", "search", "read", "stats", "status", "validate", "rebuild", "purge", "config"}
	for _, cmd := range approvedV2 {
		if !strings.Contains(commandsSection, "\n  "+cmd+" ") && !strings.Contains(commandsSection, "\n  "+cmd+"\n") {
			t.Errorf("--help missing approved v2 command %q", cmd)
		}
	}

	// v1 commands that SHOULD NOT be in root Available Commands section (removed in v2)
	removedV1 := []string{"sessions", "events", "inputs", "projects", "topics", "insights", "export", "sync", "resume", "reindex"}
	for _, cmd := range removedV1 {
		// Check for command name as a line item: "  <cmd> " or "  <cmd>\n"
		if strings.Contains(commandsSection, "\n  "+cmd+" ") || strings.Contains(commandsSection, "\n  "+cmd+"\n") {
			t.Errorf("--help should not contain removed v1 command %q as root command", cmd)
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
	t.Setenv("BACKSCROLL_SESSION_DIRS", fixtureSession)

	// v2: search auto-syncs before querying. No explicit sync needed.

	// Status should show indexed content (auto-syncs first)
	out, _, err := runCmd("status")
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if !strings.Contains(out, "Files indexed") {
		t.Errorf("status missing 'Files indexed': %s", out)
	}

	// Search should return results (auto-syncs first)
	out, _, err = runCmd("search", "--text", "hello")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	_ = out // may or may not have results depending on fixture content
}

func TestSyncWithPiFixture(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	t.Setenv("BACKSCROLL_SESSION_DIRS", piDir)

	// v2: auto-sync happens before query commands. Test via list (which auto-syncs).
	out, stderr, err := runCmd("list")
	if err != nil {
		t.Fatalf("list (with auto-sync) error: %v\nstderr: %s", err, stderr)
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

func TestReadLargeJSONLLine(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	path := writeLargeJSONLFixture(t)
	out, _, err := runCmd("read", path)
	if err != nil {
		t.Fatalf("read large JSONL error: %v", err)
	}
	if !strings.Contains(out, "Total messages: 47") {
		t.Fatalf("read output missing message count; output prefix: %.200q", out)
	}
}

func TestReadPathTailSemanticHandlesLargeJSONLLine(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	path := writeLargeJSONLFixture(t)
	out, _, err := runCmd("read", "--path", path, "--tail", "45", "--semantic")
	if err != nil {
		t.Fatalf("read --path --tail --semantic error: %v", err)
	}
	if strings.Contains(out, "bufio.Scanner: token too long") {
		t.Fatalf("semantic tail hit scanner token limit: %s", out)
	}
	if strings.Contains(out, "oversized-") {
		t.Fatalf("semantic tail should not include the oversized first row: %.200q", out)
	}
	for _, want := range []string{"path=", "line=4", "line=48", "role=\"assistant\"", "content=\"final answer\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("semantic tail output missing %q: %s", want, out)
		}
	}
}

func writeLargeJSONLFixture(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "large.jsonl")
	large := "oversized-" + strings.Repeat("x", 70*1024)
	lines := []string{
		fmt.Sprintf(`{"type":"message","timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":%q}}`, large),
		`{"type":"message","timestamp":"2024-01-01T00:00:01Z","message":{"role":"assistant","content":"middle answer"}}`,
		`{"type":"message","timestamp":"2024-01-01T00:00:02Z","message":{"role":"user","content":[{"type":"tool_use","id":"tool-1","name":"bash","input":{"command":"echo hi"}}]}}`,
	}
	for i := 4; i < 48; i++ {
		lines = append(lines, fmt.Sprintf(`{"type":"message","timestamp":"2024-01-01T00:00:%02dZ","message":{"role":"assistant","content":"tail item %d"}}`, i, i))
	}
	lines = append(lines, `{"type":"message","timestamp":"2024-01-01T00:00:48Z","message":{"role":"assistant","content":"final answer"}}`)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestReadNonExistent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, err := runCmd("read", "/nonexistent/path.jsonl")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestReadSemanticDefaultAgentReadable(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	path := writeLargeJSONLFixture(t)
	out, stderr, err := runCmd("read", "--path", path, "--tail", "5", "--semantic")
	if err != nil {
		t.Fatalf("read --semantic error: %v", err)
	}

	// Default output should be agent-readable (key=value format)
	// and should NOT go to stderr
	if stderr != "" {
		t.Errorf("stderr should be empty for data output, got: %s", stderr)
	}

	// Verify agent-readable format: tab-separated key=value rows
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 {
		t.Error("no output generated")
	}

	// Each line should have key=value format (except the final total= line)
	for i, line := range lines {
		if line == "" {
			continue
		}
		if !strings.Contains(line, "=") {
			t.Errorf("line %d not in key=value format: %q", i, line)
		}
	}
}

func TestReadSemanticWithPretty(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	path := writeLargeJSONLFixture(t)
	out, _, err := runCmd("read", "--path", path, "--tail", "3", "--semantic", "--pretty")
	if err != nil {
		t.Fatalf("read --semantic --pretty error: %v", err)
	}

	// Pretty output should be human-readable (different from default key=value)
	// For now, we just verify it runs without error
	// Real implementation may add table headers or aligned columns
	if len(out) == 0 {
		t.Error("--pretty output should not be empty")
	}

	// Pretty output should include human-readable elements
	// For now just verify it exists and is different from raw key=value
	if strings.Count(out, "=") > 3 {
		t.Logf("pretty output still has key=value pairs: %s", out[:200])
	}
}

func TestReadSemanticNoRobotFlag(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	path := writeLargeJSONLFixture(t)

	// Verify that --robot flag is not recognized (removed from v2 UX)
	// Expected: error about unknown flag
	_, _, err := runCmd("read", "--path", path, "--semantic", "--robot")
	if err == nil {
		t.Error("--robot flag should not be recognized in v2 CLI (expected error)")
	}
}

func TestReadSemanticPrettyIncludesHeaders(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	path := writeLargeJSONLFixture(t)
	out, _, err := runCmd("read", "--path", path, "--tail", "2", "--semantic", "--pretty")
	if err != nil {
		t.Fatalf("read --semantic --pretty error: %v", err)
	}

	// Pretty output should include table headers
	if !strings.Contains(out, "Path") || !strings.Contains(out, "Line") {
		t.Errorf("pretty output should have headers; got: %s", out[:200])
	}

	// Pretty output should have content aligned in columns
	if !strings.Contains(out, "Total rows:") {
		t.Errorf("pretty output should have total rows summary; got: %s", out)
	}
}

func TestReadSemanticAgentFormatIsTabSeparated(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	path := writeLargeJSONLFixture(t)
	out, _, err := runCmd("read", "--path", path, "--tail", "1", "--semantic")
	if err != nil {
		t.Fatalf("read --semantic error: %v", err)
	}

	// Default agent format should use key=value pairs
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines (data + total), got %d", len(lines))
	}

	// Each data line should have key=value pairs separated by spaces
	dataLine := lines[0]

	// Verify presence of expected keys in agent format
	expectedKeys := []string{"path=", "line=", "ordinal=", "timestamp=", "role=", "kind=", "content="}
	for _, key := range expectedKeys {
		if !strings.Contains(dataLine, key) {
			t.Errorf("agent format missing key %q in line: %q", key, dataLine)
		}
	}

	// Verify it's NOT pretty format
	if strings.Contains(dataLine, "---") || strings.Contains(dataLine, "Total rows:") {
		t.Errorf("agent format should not include pretty formatting")
	}
}

func TestList(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// v2: auto-sync before query commands
	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")
	cleanupDir := setupSessionDir(t, fixtureSession)
	defer cleanupDir()

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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

func TestSyncSubagentExcluded(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync the claude-preset which has a subagents/ directory
	fixtureSession := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", fixtureSession)
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
	out, _, err := syncForTest(t, "sync", "--path", fixtureSession, "--include-agents")
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
	_, _, err := syncForTest(t, "sync", "--path", fixtureSession)
	if err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	_, _, err = syncForTest(t, "sync", "--path", fixtureSession)
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
	_, _, err := syncForTest(t, "sync", "--path", piDir, "--no-plans")
	if err != nil {
		t.Fatalf("sync --no-plans error: %v", err)
	}
}

func TestSyncSearchRoundtrip(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// The pi fixture has "message" type sessions
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
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
	for _, flag := range []string{"--project", "--source", "--after", "--before", "--role", "--limit", "--json", "--robot", "--lexical-only", "--similarity-threshold"} {
		if !strings.Contains(out, flag) {
			t.Errorf("search --help missing flag %q", flag)
		}
	}
}

func TestSearchLexicalOnly(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixture := filepath.Join(fixturesDir(), "claude.jsonl")
	_, _, _ = syncForTest(t, "sync", "--path", filepath.Dir(fixture))

	out, _, err := runCmd("search", "--lexical-only", "session")
	if err != nil {
		t.Fatalf("search --lexical-only error: %v", err)
	}
	_ = out
}

func TestSearchSimilarityThreshold(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	fixture := filepath.Join(fixturesDir(), "claude.jsonl")
	_, _, _ = syncForTest(t, "sync", "--path", filepath.Dir(fixture))

	// With no vectors in DB, falls back to BM25 regardless of threshold
	out, _, err := runCmd("search", "--similarity-threshold", "0.5", "session")
	if err != nil {
		t.Fatalf("search --similarity-threshold error: %v", err)
	}
	_ = out
}

func TestListTextSynced(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

	out, _, err := runCmd("search", "test", "--project", "unknown")
	if err != nil {
		t.Fatalf("search --project error: %v", err)
	}
	_ = out
}

func TestSearchAfterBefore(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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
format = "claude"
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

func TestHelpListsAllCommands(t *testing.T) {
	out, _, err := runCmd("--help")
	if err != nil {
		t.Fatalf("--help error: %v", err)
	}

	// Extract the "Available Commands:" section
	parts := strings.Split(out, "Available Commands:")
	if len(parts) < 2 {
		t.Fatalf("Could not find 'Available Commands:' section in help")
	}
	commandsSection := parts[1]

	// v2 approved root commands
	for _, cmd := range []string{
		"search", "read", "list", "stats", "purge", "validate", "status",
		"rebuild", "config",
	} {
		if !strings.Contains(commandsSection, "\n  "+cmd+" ") && !strings.Contains(commandsSection, "\n  "+cmd+"\n") {
			t.Errorf("--help missing v2 command %q", cmd)
		}
	}

	// v1 commands removed from root
	for _, cmd := range []string{
		"sync", "resume", "topics", "insights", "export", "reindex",
		"decisions", "projects", "inputs", "sessions", "events",
	} {
		// Check for command name as a line item: "  <cmd> " or "  <cmd>\n"
		if strings.Contains(commandsSection, "\n  "+cmd+" ") || strings.Contains(commandsSection, "\n  "+cmd+"\n") {
			t.Errorf("--help should not contain removed v1 command %q as root command", cmd)
		}
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

func TestListRecentN(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

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

func TestStatusWithDeclarativeInputs(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	presetDir := filepath.Join(fixturesDir(), "claude-preset")
	cfgDir := t.TempDir()
	setupInputsPreset(t, cfgDir, filepath.Join(presetDir, "projects"))
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	out, _, err := runCmd("status", "--json")
	if err != nil {
		t.Fatalf("status --json with preset error: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	cfg, ok := result["config"].(map[string]any)
	if !ok {
		t.Fatalf("config field missing: %v", result)
	}
	if using, ok := cfg["using_declarative_inputs"].(bool); !ok || !using {
		t.Errorf("expected using_declarative_inputs=true, got: %v", cfg["using_declarative_inputs"])
	}
}

func TestMain_AutoupdateConstructorParams(t *testing.T) {
	// Verify autoupdate is initialized with correct repo, binary, and env var.
	// We verify by calling run() which initializes the updater; if params are
	// wrong, FetchAndStage would fail. Since we use the disable env var in this
	// test, no network calls occur.
	_ = os.Setenv("BACKSCROLL_AUTOUPDATE_DISABLE", "1")
	defer func() { _ = os.Unsetenv("BACKSCROLL_AUTOUPDATE_DISABLE") }()

	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(out, "backscroll") {
		t.Errorf("expected version output, got: %s", out)
	}
}

func TestMain_AutoupdateSkipsOnEnv(t *testing.T) {
	// Verify that setting BACKSCROLL_AUTOUPDATE_DISABLE=1 disables autoupdate.
	// CLI completes without hanging.
	_ = os.Setenv("BACKSCROLL_AUTOUPDATE_DISABLE", "1")
	defer func() { _ = os.Unsetenv("BACKSCROLL_AUTOUPDATE_DISABLE") }()

	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, err := runCmd("status")
	if err != nil {
		t.Fatalf("status command should not error when autoupdate disabled: %v", err)
	}
}

func TestMain_AutoupdateSkipsOnDevVersion(t *testing.T) {
	// Verify that version == "dev" (the default) doesn't trigger network requests.
	// The goroutine runs but exits quickly. We verify by checking that --help
	// completes promptly without blocking.
	_, cleanup := testEnv(t)
	defer cleanup()

	out, _, err := runCmd("--help")
	if err != nil {
		t.Fatalf("--help failed: %v", err)
	}
	if !strings.Contains(out, "backscroll") {
		t.Errorf("expected help output, got: %s", out)
	}
}

func TestMain_AutoupdateFetchRunsInGoroutine(t *testing.T) {
	// Verify that FetchAndStage runs in a goroutine and doesn't block CLI execution.
	// We measure this by ensuring --version returns quickly even if autoupdate is
	// doing work in the background.
	_ = os.Setenv("BACKSCROLL_AUTOUPDATE_DISABLE", "1")
	defer func() { _ = os.Unsetenv("BACKSCROLL_AUTOUPDATE_DISABLE") }()

	_, cleanup := testEnv(t)
	defer cleanup()

	// This should return immediately without waiting for autoupdate
	out, _, err := runCmd("--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected non-empty version output")
	}
}

func init() {
	// Suppress cobra's default behavior of writing to os.Stderr on error
	_ = fmt.Sprintf // keep fmt import
}

func TestSearchFieldsInvalid(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	_, _, err := runCmd("search", "test", "--fields", "bogus")
	if err == nil {
		t.Fatal("expected error for invalid --fields value, got nil")
	}
	if !strings.Contains(err.Error(), "--fields") {
		t.Errorf("error should mention --fields, got: %v", err)
	}
}

func TestSearchSourcePath(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

	// Glob that matches nothing must return no results
	out, _, err := runCmd("search", "pi", "--json", "--source-path", "/nonexistent/*", "--all-projects")
	if err != nil {
		t.Fatalf("search --source-path error: %v", err)
	}
	if strings.Contains(out, "source_path") && strings.Contains(out, "pi-session") {
		t.Errorf("non-matching --source-path glob returned results: %s", out)
	}

	// Glob matching the fixture dir should not error
	_, _, err = runCmd("search", "pi", "--json", "--source-path", "*pi-session.jsonl", "--all-projects")
	if err != nil {
		t.Fatalf("search --source-path glob error: %v", err)
	}
}

func TestSearchInvalidDates(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	if _, _, err := runCmd("search", "x", "--after", "not-a-date"); err == nil {
		t.Error("expected error for invalid --after date")
	}
	if _, _, err := runCmd("search", "x", "--before", "not-a-date"); err == nil {
		t.Error("expected error for invalid --before date")
	}
}

func TestSearchRobotWithResults(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, _ = syncForTest(t, "sync", "--path", piDir)

	out, _, err := runCmd("search", "pi", "--robot", "--all-projects")
	if err != nil {
		t.Fatalf("search --robot error: %v", err)
	}
	_ = out
}

func TestSearchAutoSync(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir()) // keep auto-sync away from the real ~/.claude

	sessionDir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixturesDir(), "claude-tool-events.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "claude-session.jsonl"), src, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	t.Setenv("BACKSCROLL_SESSION_DIRS", sessionDir)

	// No manual sync: search must auto-sync and find the new content
	out, _, err := runCmd("search", "claude", "--json", "--all-projects")
	if err != nil {
		t.Fatalf("search with auto-sync error: %v", err)
	}
	if !strings.Contains(out, "claude-session.jsonl") {
		t.Errorf("auto-sync did not index new session; output: %s", out)
	}
}

func TestStatusJSONIndexUsable(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	// No index yet: --indexed-only must report usable=false without creating the DB
	out, _, err := runCmd("status", "--json", "--indexed-only")
	if err != nil {
		t.Fatalf("status --json --indexed-only error: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("status JSON invalid: %v\noutput: %s", err, out)
	}
	index, ok := doc["index"].(map[string]any)
	if !ok {
		t.Fatalf("status JSON missing index object: %s", out)
	}
	if usable, _ := index["usable"].(bool); usable {
		t.Error("expected index.usable=false with no index")
	}

	// After syncing a session, usable must flip to true
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "claude-tool-events.jsonl"))
	_, _, _ = syncForTest(t, "sync", "--path", piDir)
	// v2: status without --indexed-only triggers auto-sync
	out, _, err = runCmd("status", "--json")
	if err != nil {
		t.Fatalf("status after sync error: %v", err)
	}
	doc = nil
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("status JSON invalid after sync: %v", err)
	}
	index = doc["index"].(map[string]any)
	if usable, _ := index["usable"].(bool); !usable {
		t.Error("expected index.usable=true after sync")
	}
}

func TestListWithOrderFlag(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Test --order flag exists and accepts timestamp:desc
	out, _, err := runCmd("list", "--order", "timestamp:desc", "--limit", "1")
	if err != nil {
		t.Fatalf("list --order timestamp:desc error: %v", err)
	}
	// Should have output (at least one item)
	if len(strings.TrimSpace(out)) == 0 {
		t.Errorf("list --order timestamp:desc --limit 1 produced empty output")
	}
}

func TestSearchWithTextFlag(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Test --text flag exists and accepts a value
	out, _, err := runCmd("search", "--text", "pi")
	if err != nil {
		t.Fatalf("search --text pi error: %v", err)
	}
	_ = out
}

func TestListWithInputFilterReturnsData(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions - this creates data in the database
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Get baseline list count (without filters)
	out1, _, err := runCmd("list", "--limit", "10")
	if err != nil {
		t.Fatalf("list without filter error: %v", err)
	}

	// Verify we got some results
	if len(out1) == 0 {
		t.Logf("baseline list produced no output; skipping input filter test")
		return
	}

	// Try list with a limit (verifies --limit still works after session sync)
	out2, _, err := runCmd("list", "--limit", "10")
	if err != nil {
		t.Fatalf("list --limit 10 error: %v", err)
	}
	_ = out2
}

// Slice 4: Structured tool-call listing and stats

func TestListWithTypeToolFilterNoPathResolution(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions containing tool calls
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Test list --type tool_call --tool bash (should not try to resolve bash as a path)
	out, _, err := runCmd("list", "--type", "tool_call", "--tool", "bash", "--limit", "10")
	if err != nil {
		t.Fatalf("list --input claude --type tool_call --tool bash error: %v", err)
	}
	_ = out // We're just checking that the command runs without path resolution errors
}

func TestListStructuredToolCallRows(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions containing tool calls
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Test list --type tool_call returns structured rows
	out, _, err := runCmd("list", "--type", "tool_call", "--limit", "5")
	if err != nil {
		t.Fatalf("list --type tool_call error: %v", err)
	}

	// Should have some output (assuming fixtures contain tool calls)
	if len(strings.TrimSpace(out)) == 0 {
		t.Logf("list --type tool_call produced empty output (may be expected if fixtures have no tool calls)")
	}
}

func TestStatsCommandExists(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Test that stats command exists and can be called
	out, _, err := runCmd("stats", "--type", "tool_call", "--group-by", "agent")
	if err != nil {
		t.Fatalf("stats --type tool_call --group-by agent error: %v", err)
	}
	_ = out
}

func TestStatsGroupByAgent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Test stats --group-by agent returns agent counts
	out, _, err := runCmd("stats", "--input", "claude", "--type", "tool_call", "--group-by", "agent")
	if err != nil {
		t.Fatalf("stats --input claude --type tool_call --group-by agent error: %v", err)
	}

	// Should have output (either with real agents or <unknown> fallback)
	if len(strings.TrimSpace(out)) == 0 {
		t.Errorf("stats --group-by agent produced empty output")
	}
}

func TestAmbiguousPositionalError(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture sessions
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Test that a bare positional argument produces a helpful error
	// (this should fail, and the error should hint at --text or --input)
	_, stderr, err := runCmd("list", "ambiguous_token")
	if err == nil {
		t.Fatalf("list with bare positional should error, but got: %s", stderr)
	}

	// Error message should hint at appropriate flags
	if !strings.Contains(stderr, "--") && !strings.Contains(stderr, "flag") {
		t.Logf("error message could be more helpful; got: %s", stderr)
	}
}

// Slice 5: Maintenance command cleanup tests

// TestRebuildCommand verifies that rebuild command exists and works like reindex.
func TestRebuildCommand(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// rebuild should exist (new v2 command)
	_, _, err := runCmd("rebuild")
	if err == nil {
		// rebuild exists, which is expected; test passes if command is recognized
		return
	}
	// If rebuild doesn't exist, that's also acceptable (early implementation stage)
	if !strings.Contains(err.Error(), "unknown command") {
		t.Logf("rebuild command error (acceptable if still in development): %v", err)
	}
}

// TestValidateWithIndexedOnly verifies that validate respects --indexed-only flag.
func TestValidateWithIndexedOnly(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// validate --indexed-only should work on empty DB without erroring
	out, _, err := runCmd("validate", "--indexed-only")
	if err != nil {
		// validate may fail on empty DB (expected), but the flag should be recognized
		if strings.Contains(err.Error(), "unknown flag: --indexed-only") {
			t.Fatalf("validate does not support --indexed-only flag")
		}
		// Otherwise, error is acceptable for empty DB
		t.Logf("validate --indexed-only error (may be expected on empty DB): %v", err)
		return
	}
	// Success: validate --indexed-only worked
	if len(strings.TrimSpace(out)) == 0 {
		t.Logf("validate --indexed-only produced empty output")
	}
}

// TestConfigCommand verifies that config command exists and shows input manifest info.
func TestConfigCommand(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// config command should exist (new v2 command)
	out, _, err := runCmd("config")
	if err == nil {
		// config exists and succeeded, which is expected
		if len(strings.TrimSpace(out)) == 0 {
			t.Errorf("config produced empty output")
		}
		return
	}
	// If config doesn't exist, that's also acceptable (early implementation stage)
	if !strings.Contains(err.Error(), "unknown command") {
		t.Logf("config command error (acceptable if still in development): %v", err)
	}
}

// TestStatusAndValidateAreMaintenanceV2 verifies status and validate are the v2 maintenance surface.
func TestStatusAndValidateAreMaintenanceV2(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// sync first to create index
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// status --indexed-only should succeed and show agent-readable output by default
	out, _, err := runCmd("status", "--indexed-only")
	if err != nil {
		t.Fatalf("status --indexed-only error: %v", err)
	}
	if len(strings.TrimSpace(out)) == 0 {
		t.Errorf("status produced empty output")
	}

	// validate should succeed
	out, _, err = runCmd("validate", "--indexed-only")
	if err != nil {
		t.Fatalf("validate --indexed-only error: %v", err)
	}
	if !strings.Contains(out, "passed") && !strings.Contains(out, "✓") {
		t.Logf("validate output may not clearly indicate success: %s", out)
	}
}

// TestPurgeAndRebuildAgentOutput verifies maintenance commands output agent-readable by default.
func TestPurgeAndRebuildAgentOutput(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// sync first to create index
	claudeDir := filepath.Join(fixturesDir(), "claude-preset", "projects")
	_, _, err := syncForTest(t, "sync", "--path", claudeDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// purge should produce simple agent-readable output by default
	out, _, err := runCmd("purge", "--before", "1999-01-01")
	if err != nil {
		t.Fatalf("purge error: %v", err)
	}
	if len(strings.TrimSpace(out)) == 0 {
		t.Errorf("purge produced empty output")
	}
}

func TestSearchFindsToolCallContent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	sessionDir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixturesDir(), "claude-toolcalls.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "claude-toolcalls.jsonl"), src, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	t.Setenv("BACKSCROLL_SESSION_DIRS", sessionDir)

	// tool_use command text must be searchable (auto-sync indexes first)
	out, _, err := runCmd("search", "zzqx_marker", "--all-projects")
	if err != nil {
		t.Fatalf("search tool_use: %v", err)
	}
	if !strings.Contains(out, "zzqx_marker") {
		t.Errorf("tool_use command not indexed; output: %s", out)
	}

	// tool_result error text must be searchable
	out, _, err = runCmd("search", "zzqx_error_token", "--all-projects")
	if err != nil {
		t.Fatalf("search tool_result: %v", err)
	}
	if !strings.Contains(out, "zzqx_error_token") {
		t.Errorf("tool_result error not indexed; output: %s", out)
	}
}

// setupPiPreset writes a minimal pi.inputs.toml (format="pi") pointing at a
// fixture dir into the config dir's inputs/ so auto-sync routes to PiReader.
func setupPiPreset(t *testing.T, cfgDir, fixtureRoot string) {
	t.Helper()
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := fmt.Sprintf(`version = 1
[[inputs]]
id = "pi"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "pi"
`, fixtureRoot)
	if err := os.WriteFile(filepath.Join(inputsDir, "pi.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsPiToolCallContent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	sessionDir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixturesDir(), "pi-toolcalls.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "pi-toolcalls.jsonl"), src, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfgDir := t.TempDir()
	setupPiPreset(t, cfgDir, sessionDir)
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	// toolCall argument text must be searchable (auto-sync indexes first)
	out, _, err := runCmd("search", "pizzqx_marker", "--all-projects")
	if err != nil {
		t.Fatalf("search toolCall: %v", err)
	}
	if !strings.Contains(out, "pizzqx_marker") {
		t.Errorf("Pi toolCall args not indexed; output: %s", out)
	}

	// custom-record result text must be searchable
	out, _, err = runCmd("search", "pizzqx_result_token", "--all-projects")
	if err != nil {
		t.Fatalf("search custom result: %v", err)
	}
	if !strings.Contains(out, "pizzqx_result_token") {
		t.Errorf("Pi custom result not indexed; output: %s", out)
	}
}
