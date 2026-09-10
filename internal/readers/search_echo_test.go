package readers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func TestDirectSearchInputBoundaries(t *testing.T) {
	for _, tc := range []struct {
		tool, command string
		want          bool
	}{
		{"Bash", "backscroll search --text orchard", true},
		{"bash", " \tbackscroll\nsearch --robot", true},
		{"Bash", "/usr/bin/backscroll search orchard", false},
		{"Bash", "env backscroll search orchard", false},
		{"Bash", "bash -lc 'backscroll search orchard'", false},
		{"Bash", "backscroll searcher orchard", false},
		{"Bash", "backscroll status", false},
		{"Bash", "echo backscroll search orchard", false},
		{"Read", "backscroll search orchard", false},
		{"Bash", "", false},
	} {
		t.Run(tc.tool+tc.command, func(t *testing.T) {
			input, _ := json.Marshal(map[string]string{"command": tc.command})
			if got := isDirectSearchInput(tc.tool, input); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
	if isDirectSearchInput("Bash", json.RawMessage(`{`)) {
		t.Fatal("malformed input classified")
	}
}

func TestClaudeSearchEchoPairingUsesIDsNotOutputShapeOrAdjacency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paired.jsonl")
	data := `{"type":"assistant","uuid":"use","message":{"role":"assistant","content":[{"type":"tool_use","id":"search","name":"Bash","input":{"command":"backscroll search orchard"}},{"type":"tool_use","id":"rg","name":"Bash","input":{"command":"rg orchard"}}]}}
{"type":"user","uuid":"ordinary-output","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"rg","content":"result_1_snippet=orchard"}]}}
{"type":"assistant","uuid":"prose","message":{"role":"assistant","content":"backscroll search orchard is documented here"}}
{"type":"user","uuid":"search-output","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"search","content":"orchard failure","is_error":true}]}}
{"type":"user","uuid":"orphan-output","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"missing","content":"result_1_snippet=orchard"}]}}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := (&ClaudeReader{}).Parse(path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"use#t0": true, "use#t1": false, "ordinary-output#r0": false, "prose": false, "search-output#r0": true, "orphan-output#r0": false}
	if len(parsed.Records) != len(want) {
		t.Fatalf("records=%d want %d", len(parsed.Records), len(want))
	}
	for _, msg := range parsed.Records {
		if msg.SearchEcho != want[msg.UUID] {
			t.Errorf("%s echo=%v want %v", msg.UUID, msg.SearchEcho, want[msg.UUID])
		}
	}
	// Same IDs in another file cannot supply a missing call.
	if err := os.WriteFile(path, []byte(`{"type":"user","uuid":"only-result","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"search","content":"orchard"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err = (&ClaudeReader{}).Parse(path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Records) != 1 || parsed.Records[0].SearchEcho {
		t.Fatal("pairing leaked across files")
	}
}
