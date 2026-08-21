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

func setupSequencesHermeticEmptyInputEnv(t *testing.T) (dbPath, cfgDir string) {
	t.Helper()
	root := t.TempDir()
	dbPath = filepath.Join(root, "backscroll.db")
	cfgDir = filepath.Join(root, "config")
	setIndexPolicyEnv(t, dbPath, cfgDir)
	return dbPath, cfgDir
}

func runPatternsSequences(t *testing.T, extraArgs ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	args := append([]string{"patterns", "--kind", "sequences"}, extraArgs...)
	err := run(&stdout, &stderr, args)
	return stdout.String(), stderr.String(), err
}

func TestPatternsSequencesCommandBasic(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t, "--json")
	if err != nil {
		t.Fatalf("patterns sequences --json failed on hermetic empty input: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if !strings.Contains(stdout, `"kind":"sequences"`) {
		t.Fatalf("patterns sequences --json missing kind marker, stdout=%q", stdout)
	}
}

func TestPatternsSequencesCommandText(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t)
	if err != nil {
		t.Fatalf("patterns sequences text failed on hermetic empty input: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "No patterns found") {
		t.Fatalf("expected no-pattern guidance in stdout, got %q", stdout)
	}
}

func TestPatternsSequencesJSON(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t, "--json")
	if err != nil {
		t.Fatalf("patterns sequences --json failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("patterns sequences --json emitted invalid JSON: %v stdout=%q", err, stdout)
	}
	if kind, _ := result["kind"].(string); kind != "sequences" {
		t.Fatalf("patterns sequences --json kind=%q, want sequences payload=%v", kind, result)
	}
}

func TestPatternsSequencesRobotFormat(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t, "--robot")
	if err != nil {
		t.Fatalf("patterns sequences --robot failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "no results") {
		t.Fatalf("patterns sequences --robot missing empty-index hint in stderr=%q", stderr)
	}
}

func TestPatternsSequencesWithFlags(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t,
		"--min-support", "5",
		"--min-length", "3",
		"--max-length", "8",
		"--limit", "10",
		"--offset", "0",
		"--json",
	)
	if err != nil {
		t.Fatalf("patterns sequences with tuning flags failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
}

func TestPatternsSequencesRobotWithData(t *testing.T) {
	_, cfgDir := setupSequencesHermeticEmptyInputEnv(t)

	sessionDir := t.TempDir()
	jsonl := `{"uuid":"u1","message":{"role":"assistant","content":{"type":"tool","name":"Read"}},"type":"message","timestamp":"2026-01-01T00:00:00Z"}
{"uuid":"u2","message":{"role":"assistant","content":{"type":"tool","name":"Write"}},"type":"message","timestamp":"2026-01-02T00:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "test.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write session fixture: %v", err)
	}

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
		t.Fatalf("mkdir inputs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "claude-test.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatalf("write input manifest: %v", err)
	}

	stdout, stderr, err := runPatternsSequences(t, "--robot", "--min-support", "1", "--min-length", "2")
	if err != nil {
		t.Fatalf("patterns sequences --robot custom-manifest fixture failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if stderr != "" && !strings.Contains(stderr, "no results") {
		t.Fatalf("patterns sequences --robot custom-manifest unexpected stderr=%q stdout=%q", stderr, stdout)
	}
}

func TestPatternsSequencesProjectFilter(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t, "--project", "myproject", "--json")
	if err != nil {
		t.Fatalf("patterns sequences --project failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
}

func TestPatternsSequencesAllProjects(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t, "--all-projects", "--json")
	if err != nil {
		t.Fatalf("patterns sequences --all-projects failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
}

func TestPatternsSequencesInvalidMinSupport(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t, "--min-support", "-1", "--json")
	if err != nil {
		t.Fatalf("patterns sequences --min-support -1 unexpectedly failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
}

func TestPatternsSequencesEmptyDBGuidance(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t)
	if err != nil {
		t.Fatalf("patterns sequences text failed on empty DB: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	for _, want := range []string{"no results", "--all-projects", "backscroll status"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("missing guidance %q in stderr=%q", want, stderr)
		}
	}
}

func TestPatternsSequencesFullEndToEnd(t *testing.T) {
	_, cfgDir := setupSequencesHermeticEmptyInputEnv(t)

	sessionDir := t.TempDir()
	jsonl := `{"uuid":"u1","message":{"role":"assistant","content":"test output"},"type":"message","timestamp":"2026-01-01T00:00:00Z"}
{"uuid":"u2","message":{"role":"assistant","content":{"type":"tool","name":"Read"}},"type":"message","timestamp":"2026-01-01T00:00:01Z"}
{"uuid":"u3","message":{"role":"assistant","content":{"type":"tool","name":"Write"}},"type":"message","timestamp":"2026-01-01T00:00:02Z"}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "test.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write session fixture: %v", err)
	}

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
		t.Fatalf("mkdir inputs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "claude-test.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatalf("write input manifest: %v", err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "text", args: nil},
		{name: "json", args: []string{"--json"}},
		{name: "robot", args: []string{"--robot"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--min-support", "1"}, tc.args...)
			stdout, stderr, err := runPatternsSequences(t, args...)
			if err != nil {
				t.Fatalf("patterns sequences %s failed: %v stdout=%q stderr=%q", tc.name, err, stdout, stderr)
			}
			if tc.name == "json" && !strings.Contains(stdout, `"kind":"sequences"`) {
				t.Fatalf("patterns sequences json output missing kind marker: %q", stdout)
			}
		})
	}
}

func TestPatternsSequencesDefaultMinSupport(t *testing.T) {
	setupSequencesHermeticEmptyInputEnv(t)
	stdout, stderr, err := runPatternsSequences(t, "--min-support", "0", "--json")
	if err != nil {
		t.Fatalf("patterns sequences --min-support 0 failed: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if !strings.Contains(stdout, `"kind":"sequences"`) {
		t.Fatalf("patterns sequences --min-support 0 missing JSON payload: %q", stdout)
	}
}

// TestPatternsSequencesMalformedCategoriesReturnsLoadError asserts malformed
// categories with a valid version field fail the command through the genuine
// load chain (getVersion succeeds, parseMapper regexp compile fails).
func TestPatternsSequencesMalformedCategoriesReturnsLoadError(t *testing.T) {
	dbPath, cfgDir := setupSequencesHermeticEmptyInputEnv(t)

	categoriesDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(categoriesDir, 0o755); err != nil {
		t.Fatalf("mkdir categories dir: %v", err)
	}
	badCategories := `version = 2
[[rule]]
tool = "Bash"
pattern = "["
category = "BROKEN"
`
	if err := os.WriteFile(filepath.Join(categoriesDir, "categories.toml"), []byte(badCategories), 0o644); err != nil {
		t.Fatalf("write categories fixture: %v", err)
	}

	stdout, stderr, err := runPatternsSequences(t)
	if err == nil {
		t.Fatalf("patterns sequences succeeded with invalid categories regex; stdout=%q stderr=%q db=%s", stdout, stderr, dbPath)
	}
	errText := err.Error()
	for _, want := range []string{"load sequences", "load categories", "compile pattern"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("patterns sequences error missing %q: %v", want, err)
		}
	}
}
