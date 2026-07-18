package categories

import (
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
	if m.Version() != 1 {
		t.Errorf("version: %d, want 1", m.Version())
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
