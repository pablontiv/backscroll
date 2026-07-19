package categories

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestCategorizeExactToolMatch(t *testing.T) {
	m := &Mapper{
		version: 1,
		rules: []Rule{
			{Tool: "Read", Category: "FILE_READ"},
			{Tool: "Bash", Category: "SHELL_OTHER"},
		},
	}
	if cat := m.Categorize("Read", ""); cat != "FILE_READ" {
		t.Errorf("Read: %q, want FILE_READ", cat)
	}
}

func TestCategorizePatternMatch(t *testing.T) {
	re := regexp.MustCompile(`^(go|Bash) (test|vet|build)`)
	m := &Mapper{
		version: 1,
		rules: []Rule{
			{Pattern: re, Category: "GO_EXEC"},
		},
	}
	if cat := m.Categorize("Bash", "test"); cat != "GO_EXEC" {
		t.Errorf("Bash test: %q, want GO_EXEC", cat)
	}
}

func TestCategorizeFallthrough(t *testing.T) {
	m := &Mapper{version: 1, rules: nil}
	if cat := m.Categorize("CustomTool", ""); cat != "CustomTool" {
		t.Errorf("CustomTool: %q, want CustomTool", cat)
	}
}

func TestLoadEmbedded(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.Version() != 2 {
		t.Errorf("version: %d, want 2", m.Version())
	}
	if cat := m.Categorize("Read", ""); cat != "FILE_READ" {
		t.Errorf("embedded Read: %q, want FILE_READ", cat)
	}
}

func TestCategorizeToolPrecedence(t *testing.T) {
	// Exact tool match should have higher precedence than pattern match
	re := regexp.MustCompile(`^Bash`)
	m := &Mapper{
		version: 1,
		rules: []Rule{
			{Tool: "Bash", Category: "EXACT_TOOL"},
			{Pattern: re, Category: "PATTERN_MATCH"},
		},
	}
	if cat := m.Categorize("Bash", "test"); cat != "EXACT_TOOL" {
		t.Errorf("Bash: %q, want EXACT_TOOL", cat)
	}
}

func TestCategorizeWithToolAndPattern(t *testing.T) {
	// Tool match with pattern refinement
	re := regexp.MustCompile(`^Bash git`)
	m := &Mapper{
		version: 1,
		rules: []Rule{
			{Tool: "Bash", Pattern: re, Category: "GIT"},
			{Tool: "Bash", Category: "SHELL"},
		},
	}
	if cat := m.Categorize("Bash", "git status"); cat != "GIT" {
		t.Errorf("Bash git status: %q, want GIT", cat)
	}
	if cat := m.Categorize("Bash", "echo test"); cat != "SHELL" {
		t.Errorf("Bash echo test: %q, want SHELL", cat)
	}
}

func TestCategorizeEmptyCommandHead(t *testing.T) {
	m := &Mapper{
		version: 1,
		rules: []Rule{
			{Tool: "Read", Category: "FILE_READ"},
		},
	}
	if cat := m.Categorize("Read", ""); cat != "FILE_READ" {
		t.Errorf("Read with empty command: %q, want FILE_READ", cat)
	}
}

func TestCategorizeComplexPattern(t *testing.T) {
	re := regexp.MustCompile(`^(python|ruby) (test|check)`)
	m := &Mapper{
		version: 1,
		rules: []Rule{
			{Pattern: re, Category: "SCRIPT_TEST"},
		},
	}
	if cat := m.Categorize("python", "test main"); cat != "SCRIPT_TEST" {
		t.Errorf("python test: %q, want SCRIPT_TEST", cat)
	}
	if cat := m.Categorize("ruby", "check"); cat != "SCRIPT_TEST" {
		t.Errorf("ruby check: %q, want SCRIPT_TEST", cat)
	}
	if cat := m.Categorize("python", "build"); cat != "python" {
		t.Errorf("python build: %q, want python (fallthrough)", cat)
	}
}

func TestMapperVersion(t *testing.T) {
	m := &Mapper{version: 42}
	if v := m.Version(); v != 42 {
		t.Errorf("version: %d, want 42", v)
	}
}

func TestLoadConfigDir(t *testing.T) {
	// Test with BACKSCROLL_CONFIG_DIR set
	tmpDir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", tmpDir)

	// configPath should use BACKSCROLL_CONFIG_DIR
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	expected := filepath.Join(tmpDir, "backscroll", "inputs", "categories.toml")
	if path != expected {
		t.Errorf("path: %q, want %q", path, expected)
	}
}

func TestLoadConfigDirWithHome(t *testing.T) {
	// Test with home-based path
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("BACKSCROLL_CONFIG_DIR", "")

	// configPath should use HOME/.config
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	expected := filepath.Join(tmpDir, ".config", "backscroll", "inputs", "categories.toml")
	if path != expected {
		t.Errorf("path: %q, want %q", path, expected)
	}
}

// TestCategorizeV2Categories tests new v2 categories are available and map correctly
func TestCategorizeV2Categories(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	tests := []struct {
		tool        string
		commandHead string
		want        string
	}{
		{"Bash", "cd /tmp", "NAV"},
		{"Bash", "rg pattern", "SEARCH"},
		{"Bash", "grep text file", "SEARCH"},
		{"Bash", "fd name", "SEARCH"},
		{"Bash", "find . -type f", "SEARCH"},
		{"Bash", "ls -la", "FILE_INSPECT"},
		{"Bash", "cat file.txt", "FILE_INSPECT"},
		{"Bash", "eza -l", "FILE_INSPECT"},
		{"Bash", "bat file", "FILE_INSPECT"},
		{"Bash", "sd old new file", "TEXT_TRANSFORM"},
		{"Bash", "jq .field", "TEXT_TRANSFORM"},
		{"Bash", "awk '{print $1}'", "TEXT_TRANSFORM"},
		{"Bash", "sed 's/a/b/'", "TEXT_TRANSFORM"},
		{"Bash", "sqlite3 db.db", "DB"},
		{"Bash", "gentle-ai review", "REVIEW_TOOL"},
		{"Bash", "just test", "TASK_RUNNER"},
		{"Bash", "echo hello", "SHELL_STATE"},
		{"Bash", "export VAR=value", "SHELL_STATE"},
		{"Bash", "mkdir dir", "FS_OP"},
		{"Bash", "mv old new", "FS_OP"},
		{"Bash", "git status", "GIT"},
		{"Bash", "gh pr list", "GIT"},
		{"go", "test ./...", "GO_EXEC"},
		{"Bash", "unknown command", "SHELL_OTHER"},
	}

	for _, tt := range tests {
		got := m.Categorize(tt.tool, tt.commandHead)
		if got != tt.want {
			t.Errorf("Categorize(%q, %q) = %q, want %q", tt.tool, tt.commandHead, got, tt.want)
		}
	}
}

// TestCategorizeV2Precedence verifies that earlier rules take precedence (first match wins)
func TestCategorizeV2Precedence(t *testing.T) {
	m, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// "cd" should map to NAV, not SHELL_OTHER (even though it matches ^Bash)
	if cat := m.Categorize("Bash", "cd /path"); cat != "NAV" {
		t.Errorf("cd: %q, want NAV (not SHELL_OTHER)", cat)
	}

	// "git status" should map to GIT, not SHELL_OTHER
	if cat := m.Categorize("Bash", "git status"); cat != "GIT" {
		t.Errorf("git: %q, want GIT (not SHELL_OTHER)", cat)
	}

	// "ls" should map to FILE_INSPECT, not SHELL_OTHER
	if cat := m.Categorize("Bash", "ls"); cat != "FILE_INSPECT" {
		t.Errorf("ls: %q, want FILE_INSPECT (not SHELL_OTHER)", cat)
	}
}

// TestStaleConfigFallsbackToEmbedded tests that a v1 config file falls back to embedded v2
func TestStaleConfigFallsbackToEmbedded(t *testing.T) {
	// Write a v1 config file to a temp directory
	tmpDir := t.TempDir()
	t.Setenv("BACKSCROLL_CONFIG_DIR", tmpDir)

	// Create the directory structure
	configDir := filepath.Join(tmpDir, "backscroll", "inputs")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a v1 config file
	v1ConfigPath := filepath.Join(configDir, "categories.toml")
	v1ConfigContent := `version = 1

[[rule]]
pattern = "^Bash"
category = "SHELL_OTHER"
`
	if err := os.WriteFile(v1ConfigPath, []byte(v1ConfigContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load should use embedded v2 instead
	m, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Verify that v2 rules apply (cd → NAV is a v2 rule)
	if cat := m.Categorize("Bash", "cd /path"); cat != "NAV" {
		t.Errorf("cd: %q, want NAV (v2 rule); config was not replaced with embedded default", cat)
	}
}
