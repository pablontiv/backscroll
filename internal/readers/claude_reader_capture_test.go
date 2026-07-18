package readers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

const captureFixture = `{"type":"user","uuid":"aaa-1","timestamp":"2026-01-01T10:00:00Z","cwd":"/w","message":{"role":"user","content":"Caveat: x Request interrupted no, te pedi otra cosa"}}
{"type":"assistant","uuid":"aaa-2","timestamp":"2026-01-01T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"running"},{"type":"tool_use","id":"tu-9","name":"Bash","input":{"command":"go test ./...","description":"run tests"}}]}}
{"type":"user","uuid":"aaa-3","timestamp":"2026-01-01T10:00:09Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-9","content":"FAIL","is_error":true}]}}
`

func TestClaudeReaderRichCapture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(captureFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := (&ClaudeReader{}).Parse(path, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pf.Records) != 4 { // user text, assistant text, tool_use, tool_result
		t.Fatalf("want 4 records, got %d: %+v", len(pf.Records), pf.Records)
	}

	user := pf.Records[0]
	if user.UUID != "aaa-1" {
		t.Errorf("user uuid = %q, want aaa-1", user.UUID)
	}
	if !user.WasInterrupted {
		t.Error("interrupt marker must be captured before CleanContent strips it")
	}

	var toolUse, toolResult *models.Message
	for i := range pf.Records {
		m := &pf.Records[i]
		switch {
		case m.ToolName == "Bash":
			toolUse = m
		case m.IsError != nil && m.ToolName == "":
			toolResult = m
		}
	}
	if toolUse == nil {
		t.Fatal("no tool_use message captured")
	}
	if toolUse.UUID != "aaa-2#t1" {
		t.Errorf("tool_use uuid = %q, want aaa-2#t1", toolUse.UUID)
	}
	if toolUse.CommandHead != "go" {
		t.Errorf("command_head = %q, want go", toolUse.CommandHead)
	}
	if toolUse.IsError == nil || !*toolUse.IsError {
		t.Error("tool_use is_error must be paired from the later tool_result (cross-record)")
	}
	if toolResult == nil || toolResult.UUID != "aaa-3#r0" {
		t.Fatalf("tool_result missing or wrong uuid: %+v", toolResult)
	}
}
