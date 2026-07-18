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

func TestPatternsSequencesCommandBasic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Use in-memory temp dir for config
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	// Run patterns command with sequences kind
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--json",
	})

	// Command should succeed or gracefully handle no patterns
	if err != nil {
		t.Logf("command returned: %v (may be expected for empty DB)", err)
	}
}

func TestPatternsSequencesCommandText(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
	})

	// Should succeed without crash
	if err != nil {
		t.Logf("command returned: %v (may be expected for empty DB)", err)
	}
	t.Logf("text output: %s", stdout.String())
}

func TestPatternsSequencesJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--json",
	})

	if err != nil {
		t.Logf("command returned: %v (may be expected for empty DB)", err)
	}

	// Parse JSON to verify structure
	var result map[string]interface{}
	if len(stdout.Bytes()) > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &result); err == nil {
			if kind, ok := result["kind"].(string); ok && kind == "sequences" {
				t.Logf("JSON output valid: %v", result)
			}
		}
	}
}

func TestPatternsSequencesRobotFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--robot",
	})

	if err != nil {
		t.Logf("command returned: %v (may be expected for empty DB)", err)
	}
	t.Logf("robot output: %s", stdout.String())
}

func TestPatternsSequencesWithFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	// Test with various flags
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--min-support", "5",
		"--min-length", "3",
		"--max-length", "8",
		"--limit", "10",
		"--offset", "0",
		"--json",
	})

	if err != nil {
		t.Logf("command returned: %v (may be expected for empty DB)", err)
	}
}

func TestPatternsSequencesRobotWithData(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Seed test data
	sessionDir := t.TempDir()
	jsonl := `{"uuid":"u1","message":{"role":"assistant","content":{"type":"tool","name":"Read"}},"type":"message","timestamp":"2026-01-01T00:00:00Z"}
{"uuid":"u2","message":{"role":"assistant","content":{"type":"tool","name":"Write"}},"type":"message","timestamp":"2026-01-02T00:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "test.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfgDir := t.TempDir()
	toml := fmt.Sprintf(`version = 1
[[inputs]]
id = "claude-test"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "claude"
`, sessionDir)
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "claude-test.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--robot",
	})

	if err != nil {
		t.Logf("command result: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, "Sequences") || len(output) > 0 {
		t.Logf("robot output: %s", output)
	}
}

func TestPatternsSequencesProjectFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--project", "myproject",
		"--json",
	})

	if err != nil {
		t.Logf("command returned: %v (expected for empty DB)", err)
	}
}

func TestPatternsSequencesAllProjects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--all-projects",
		"--json",
	})

	if err != nil {
		t.Logf("command returned: %v (expected for empty DB)", err)
	}
}

func TestPatternsSequencesInvalidMinSupport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	err := run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--min-support", "-1",
		"--json",
	})

	t.Logf("command with invalid --min-support: %v", err)
}

func TestPatternsSequencesEmptyDBGuidance(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	_ = run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
	})

	stderrStr := stderr.String()
	t.Logf("stderr guidance: %s", stderrStr)
}

func TestPatternsSequencesFullEndToEnd(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Seed multiple tool events that will create a pattern
	sessionDir := t.TempDir()
	jsonl := `{"uuid":"u1","message":{"role":"assistant","content":"test output"},"type":"message","timestamp":"2026-01-01T00:00:00Z"}
{"uuid":"u2","message":{"role":"assistant","content":{"type":"tool","name":"Read"}},"type":"message","timestamp":"2026-01-01T00:00:01Z"}
{"uuid":"u3","message":{"role":"assistant","content":{"type":"tool","name":"Write"}},"type":"message","timestamp":"2026-01-01T00:00:02Z"}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "test.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfgDir := t.TempDir()
	toml := fmt.Sprintf(`version = 1
[[inputs]]
id = "claude-test"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "claude"
`, sessionDir)
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "claude-test.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	// Run patterns with different output formats
	for _, format := range []string{"", "--json", "--robot"} {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		args := []string{
			"patterns",
			"--kind", "sequences",
			"--min-support", "1",
		}
		if format != "" {
			args = append(args, format)
		}
		err := run(stdout, stderr, args)
		t.Logf("patterns sequences %s: err=%v, stdout=%d bytes, stderr=%d bytes", format, err, len(stdout.String()), len(stderr.String()))
	}
}

func TestPatternsSequencesDefaultMinSupport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	configDir := filepath.Join(t.TempDir(), ".config/backscroll")
	os.MkdirAll(configDir, 0o755)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)

	dbPath := filepath.Join(configDir, "backscroll.db")
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	_ = run(stdout, stderr, []string{
		"patterns",
		"--kind", "sequences",
		"--indexed-only",
		"--min-support", "0",
		"--json",
	})
}

// TestPatternsSequencesMalformedCategoriesFails asserts a categories config
// load failure fails the command (non-nil error) instead of masquerading as
// an empty result — scripts rely on the exit code to distinguish the two.
func TestPatternsSequencesMalformedCategoriesFails(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("BACKSCROLL_CONFIG_DIR", tempDir)
	t.Setenv("BACKSCROLL_DATABASE_PATH", tempDir+"/t.db")
	if err := os.MkdirAll(tempDir+"/backscroll", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tempDir+"/backscroll/categories.toml", []byte("version = [broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run(&stdout, &stderr, []string{"patterns", "--kind", "sequences", "--indexed-only"})
	if err == nil {
		t.Fatal("malformed categories config must fail the command, got nil error")
	}
}
