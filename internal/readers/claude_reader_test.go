package readers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/sync"
)

func writeClaudeFixture(t *testing.T, lines string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClaudeReader_TextAndCwd(t *testing.T) {
	line := `{"type":"user","timestamp":"2024-01-01T00:00:00Z","cwd":"/home/me/proj","message":{"role":"user","content":"hello world"}}` + "\n"
	p := writeClaudeFixture(t, line)
	r := &ClaudeReader{}
	pf, err := r.Parse(p, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Cwd != "/home/me/proj" {
		t.Errorf("Cwd = %q, want /home/me/proj", pf.Cwd)
	}
	if len(pf.Records) != 1 || pf.Records[0].Content != "hello world" {
		t.Fatalf("records = %+v", pf.Records)
	}
	if pf.Records[0].ContentType != "text" {
		t.Errorf("ContentType = %q, want text", pf.Records[0].ContentType)
	}
}

func TestClaudeReader_SkipsNoiseAndMeta(t *testing.T) {
	lines := `{"type":"system-reminder","timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"x"}}` + "\n" +
		`{"type":"user","isMeta":true,"timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"y"}}` + "\n"
	p := writeClaudeFixture(t, lines)
	pf, err := (&ClaudeReader{}).Parse(p, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("records = %d, want 0", len(pf.Records))
	}
}

func TestClaudeReader_Name(t *testing.T) {
	if (&ClaudeReader{}).Name() != "claude" {
		t.Error("Name != claude")
	}
}

func TestClaudeReader_CapturesToolUseAndResult(t *testing.T) {
	lines := `{"type":"assistant","timestamp":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"running it"},{"type":"tool_use","name":"Bash","input":{"command":"go test ./...","description":"run tests"}}]}}` + "\n" +
		`{"type":"user","timestamp":"2024-01-01T00:00:01Z","message":{"role":"user","content":[{"type":"tool_result","content":"FAIL: build broken","is_error":true}]}}` + "\n"
	p := writeClaudeFixture(t, lines)
	pf, err := (&ClaudeReader{}).Parse(p, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}

	var gotText, gotToolInput, gotToolErr bool
	for _, m := range pf.Records {
		switch {
		case m.ContentType == "text" && m.Content == "running it":
			gotText = true
		case m.ContentType == "tool" && contains(m.Content, "command=go test ./...") && contains(m.Content, "Bash"):
			gotToolInput = true
		case m.ContentType == "tool" && contains(m.Content, "error:") && contains(m.Content, "FAIL: build broken"):
			gotToolErr = true
		}
	}
	if !gotText {
		t.Error("missing text message")
	}
	if !gotToolInput {
		t.Error("missing tool_use input message")
	}
	if !gotToolErr {
		t.Error("missing tool_result error message")
	}
}

// TestExtractExitCodeBeforeTruncation verifies that exit codes are extracted
// from full tool_result text before truncation occurs (Q2 task).
func TestExtractExitCodeBeforeTruncation(t *testing.T) {
	tests := []struct {
		name              string
		toolResultText    string
		toolName          string
		wantExitCode      *int
		wantTextTruncated bool
	}{
		{
			name:              "exit code on final line, text >4000 runes",
			toolResultText:    strings.Repeat("x", 3950) + "\nexit code 42",
			toolName:          "Bash",
			wantExitCode:      ptrInt(42),
			wantTextTruncated: true,
		},
		{
			name:              "exit code extracted before truncation",
			toolResultText:    "error\nExit Code: 1",
			toolName:          "Bash",
			wantExitCode:      ptrInt(1),
			wantTextTruncated: false,
		},
		{
			name:              "no exit code",
			toolResultText:    "unknown error output",
			toolName:          "Bash",
			wantExitCode:      nil,
			wantTextTruncated: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate parsing a tool result before truncation
			exitCode := sync.ExtractExitCode(tt.toolResultText, tt.toolName)

			if (exitCode == nil && tt.wantExitCode != nil) ||
				(exitCode != nil && tt.wantExitCode == nil) ||
				(exitCode != nil && *exitCode != *tt.wantExitCode) {
				t.Errorf("ExitCode mismatch: got %v, want %v", exitCode, tt.wantExitCode)
			}
		})
	}
}

func ptrInt(i int) *int { return &i }

// Test cases for commandHead() VAR= prefix stripping (RED test - task 4.1)
func TestCommandHeadVarPrefixStripping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple command",
			input:    "go test ./internal/...",
			expected: "go",
		},
		{
			name:     "VAR= single prefix",
			input:    "SP=/path/to/code; go test ./internal/...",
			expected: "go",
		},
		{
			name:     "VAR= multiple prefixes",
			input:    "FOO=1 BAR=2 BAZ=3; rg --type go error",
			expected: "rg",
		},
		{
			name:     "VAR=value without semicolon",
			input:    "PYTHONPATH=/app python script.py",
			expected: "python",
		},
		{
			name:     "only assignment no command",
			input:    "MY_VAR=some_value",
			expected: "",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "command with equals in args",
			input:    "go test --args=value ./...",
			expected: "go",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, _ := json.Marshal(map[string]string{"command": tt.input})
			result := commandHead(inputJSON)
			if result != tt.expected {
				t.Errorf("commandHead(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
