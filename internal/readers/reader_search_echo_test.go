package readers

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func TestCodexDirectSearchCallBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, tool, arguments string
		want                  bool
	}{
		{"exec_command direct", "exec_command", `{"cmd":"backscroll search --text orchard"}`, true},
		{"exec_command whitespace", "exec_command", `{"cmd":" \tbackscroll\nsearch --robot"}`, true},
		{"exec_command absolute path", "exec_command", `{"cmd":"/usr/bin/backscroll search orchard"}`, false},
		{"exec_command env wrapper", "exec_command", `{"cmd":"env backscroll search orchard"}`, false},
		{"exec_command nested shell", "exec_command", `{"cmd":"bash -lc 'backscroll search orchard'"}`, false},
		{"exec_command other subcommand", "exec_command", `{"cmd":"backscroll status"}`, false},
		{"exec_command prefix word", "exec_command", `{"cmd":"backscroll searcher orchard"}`, false},
		{"shell argv lc", "shell", `{"command":["bash","-lc","backscroll search --text orchard"]}`, true},
		{"shell argv c", "shell", `{"command":["/bin/zsh","-c","backscroll search orchard"]}`, true},
		{"shell argv extra element", "shell", `{"command":["bash","-lc","backscroll search orchard","extra"]}`, false},
		{"shell argv other flag", "shell", `{"command":["bash","-x","backscroll search orchard"]}`, false},
		{"shell argv env inside", "shell", `{"command":["bash","-lc","env backscroll search orchard"]}`, false},
		{"shell argv direct without shell", "shell", `{"command":["backscroll","search","orchard"]}`, false},
		{"shell string command", "shell", `{"command":"backscroll search orchard"}`, false},
		{"other tool", "apply_patch", `{"cmd":"backscroll search orchard"}`, false},
		{"malformed", "exec_command", `{`, false},
		{"empty", "exec_command", `{"cmd":""}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCodexDirectSearchCall(tc.tool, tc.arguments); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestCodexSearchEchoPairingUsesCallIDsNotOutputShapeOrAdjacency(t *testing.T) {
	path := codexTestFile(t, strings.Join([]string{
		`{"timestamp":"2026-09-01T12:00:00Z","type":"session_meta","payload":{"id":"s","cwd":"/synthetic"}}`,
		`{"timestamp":"2026-09-01T12:00:01Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"search","arguments":"{\"cmd\":\"backscroll search orchard\"}"}}`,
		`{"timestamp":"2026-09-01T12:00:02Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","call_id":"rg","arguments":"{\"cmd\":\"rg orchard\"}"}}`,
		`{"timestamp":"2026-09-01T12:00:03Z","type":"response_item","payload":{"type":"function_call_output","call_id":"rg","output":"result_1_snippet=orchard"}}`,
		`{"timestamp":"2026-09-01T12:00:04Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"backscroll search orchard is documented here"}]}}`,
		`{"timestamp":"2026-09-01T12:00:05Z","type":"response_item","payload":{"type":"function_call_output","call_id":"search","output":"result_1_snippet=orchard"}}`,
		`{"timestamp":"2026-09-01T12:00:06Z","type":"response_item","payload":{"type":"function_call_output","call_id":"missing","output":"result_1_snippet=orchard"}}`,
		`{"timestamp":"2026-09-01T12:00:07Z","type":"response_item","payload":{"type":"function_call","name":"shell","call_id":"argv","arguments":"{\"command\":[\"bash\",\"-lc\",\"backscroll search orchard\"]}"}}`,
		`{"timestamp":"2026-09-01T12:00:08Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"argv","output":[{"type":"input_text","text":"result_1_snippet=orchard"}]}}`,
	}, "\n")+"\n")
	parsed, err := (&CodexReader{}).Parse(path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	want := []bool{true, false, false, false, true, false, true, true}
	if len(parsed.Records) != len(want) {
		t.Fatalf("records=%d want %d: %+v", len(parsed.Records), len(want), parsed.Records)
	}
	for i, msg := range parsed.Records {
		if msg.SearchEcho != want[i] {
			t.Errorf("record %d %q echo=%v want %v", i, msg.Content, msg.SearchEcho, want[i])
		}
	}
	// An output whose call lives in another file is never marked.
	if err := os.WriteFile(path, []byte(`{"timestamp":"2026-09-01T12:00:09Z","type":"response_item","payload":{"type":"function_call_output","call_id":"search","output":"orchard"}}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err = (&CodexReader{}).Parse(path, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Records) != 1 || parsed.Records[0].SearchEcho {
		t.Fatal("pairing leaked across files")
	}
}

func TestOpenCodeDirectSearchPartMarksInputAndOutputOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	insert := func(id string, ts int64, part any) {
		t.Helper()
		data, _ := json.Marshal(part)
		if _, err := db.Exec(`INSERT INTO message VALUES (?, ?, ?, ?, ?)`, id, "s", ts, ts, `{"role":"assistant"}`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO part VALUES (?, ?, ?, ?, ?, ?)`, "p-"+id, id, "s", ts, ts, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	tool := func(name, command, output string) map[string]any {
		return map[string]any{"type": "tool", "tool": name, "state": map[string]any{"input": map[string]any{"command": command, "description": "d"}, "output": output}}
	}
	insert("search", 1000, tool("bash", "backscroll search orchard", "result_1_snippet=orchard"))
	insert("rg", 2000, tool("bash", "rg orchard", "result_1_snippet=orchard"))
	insert("wrapper", 3000, tool("bash", "env backscroll search orchard", "result_1_snippet=orchard"))
	insert("other-tool", 4000, tool("read", "backscroll search orchard", "result_1_snippet=orchard"))
	insert("prose", 5000, map[string]any{"type": "text", "text": "backscroll search orchard is documented here"})
	insert("no-output", 6000, map[string]any{"type": "tool", "tool": "bash", "state": map[string]any{"input": map[string]any{"command": "backscroll search orchard"}}})

	parsed, err := (&OpenCodeReader{}).Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"bash command=backscroll search orchard description=d":     true,
		"bash command=rg orchard description=d":                    false,
		"bash command=env backscroll search orchard description=d": false,
		"read command=backscroll search orchard description=d":     false,
		"backscroll search orchard is documented here":             false,
		"bash command=backscroll search orchard":                   true,
	}
	var outputs int
	for _, msg := range parsed.Records {
		if msg.Content == "result_1_snippet=orchard" {
			outputs++
			continue
		}
		w, ok := want[msg.Content]
		if !ok {
			t.Errorf("unexpected record %q", msg.Content)
			continue
		}
		if msg.SearchEcho != w {
			t.Errorf("%q echo=%v want %v", msg.Content, msg.SearchEcho, w)
		}
	}
	if outputs != 4 {
		t.Fatalf("outputs=%d want 4", outputs)
	}
	var marked int
	for _, msg := range parsed.Records {
		if msg.Content == "result_1_snippet=orchard" && msg.SearchEcho {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("marked outputs=%d want exactly the direct search's own output", marked)
	}
}
