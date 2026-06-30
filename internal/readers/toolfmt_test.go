package readers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSerializeToolInput_Object(t *testing.T) {
	in := json.RawMessage(`{"command":"go test ./...","description":"run tests"}`)
	got := SerializeToolInput("bash", in)
	for _, want := range []string{"bash", "command=go test ./...", "description=run tests"} {
		if !strings.Contains(got, want) {
			t.Errorf("SerializeToolInput = %q, missing %q", got, want)
		}
	}
}

func TestSerializeToolOutput_String(t *testing.T) {
	got := SerializeToolOutput(json.RawMessage(`"exit code 1: build failed"`))
	if got != "exit code 1: build failed" {
		t.Errorf("got %q", got)
	}
}

func TestSerializeToolOutput_ArrayText(t *testing.T) {
	got := SerializeToolOutput(json.RawMessage(`[{"type":"text","text":"line one"},{"type":"text","text":"line two"}]`))
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Errorf("got %q", got)
	}
}

func TestSerialize_Truncates(t *testing.T) {
	big := strings.Repeat("x", MaxToolTextLen*2)
	got := SerializeToolOutput(json.RawMessage(`"` + big + `"`))
	if len([]rune(got)) != MaxToolTextLen {
		t.Errorf("truncated len = %d, want %d", len([]rune(got)), MaxToolTextLen)
	}
}
