package readers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
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
