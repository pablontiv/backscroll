package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/readers"
	"github.com/pablontiv/backscroll/internal/storage"
)

func codexToolRecord(t *testing.T, payload map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"type": "response_item", "timestamp": "2026-09-01T12:00:01Z", "payload": payload})
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func TestCodexNativeEventsPatternsAppendAndReplay(t *testing.T) {
	dbPath, _ := testEnv(t)
	root := t.TempDir()
	source := filepath.Join(root, "rollout.jsonl")
	writeFile(t, filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs", "codex.inputs.toml"), fmt.Sprintf(`version = 1
[[inputs]]
id = "codex"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "codex"
`, root))
	run := func(args ...string) string {
		t.Helper()
		out, stderr, err := runCmd(args...)
		if err != nil {
			t.Fatalf("%v: %v %s", args, err, stderr)
		}
		return out
	}
	text := `{"type":"session_meta","payload":{"id":"native-session","cwd":"/synthetic/codex-tools-project"}}` + "\n"
	text += codexToolRecord(t, map[string]any{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "codexechoquartz durable original prose"}}})
	calls := []map[string]any{
		{"type": "function_call", "call_id": "success", "name": "exec_command", "arguments": `{"cmd":"go test success"}`},
		{"type": "function_call", "call_id": "failure", "name": "shell", "arguments": `{"command":["sh","-lc","make test"]}`},
		{"type": "custom_tool_call", "call_id": "patch", "name": "apply_patch", "input": "synthetic patch"},
		{"type": "custom_tool_call", "call_id": "unknown", "name": "apply_patch", "input": "unknown patch"},
		{"type": "custom_tool_call", "call_id": "wrapper", "name": "exec", "input": `await tool.bash({command:"false"})`},
		{"type": "custom_tool_call", "call_id": "wrappedunknown", "name": "exec", "input": `await tool.bash({command:"true"})`},
		{"type": "function_call", "call_id": "long", "name": "exec_command", "arguments": `{"cmd":"npm test"}`},
		{"type": "function_call", "call_id": "pending", "name": "exec_command", "arguments": `{"cmd":"printf pending"}`},
		{"type": "function_call", "call_id": "echo", "name": "exec_command", "arguments": `{"cmd":"backscroll search --text codexechoquartz --all-projects --robot --fields minimal --max-tokens 0"}`},
	}
	for _, call := range calls {
		text += codexToolRecord(t, call)
	}
	writeFile(t, source, text)
	// Capture real robot search output; the result must remain a paired echo,
	// not a synthetic imitation of another command's payload.
	echoOutput := run("search", "--text", "codexechoquartz", "--all-projects", "--robot", "--fields", "minimal", "--max-tokens", "0")
	if !strings.Contains(echoOutput, "codexechoquartz") {
		t.Fatalf("real echo fixture: %s", echoOutput)
	}
	results := []map[string]any{
		{"type": "custom_tool_call_output", "call_id": "wrappedunknown", "output": `{"result":{"is_error":false},"metadata":{"exit_code":0}}`},
		{"type": "function_call_output", "call_id": "failure", "output": "Process exited with code 7\nOutput:\nerror: compile failed"},
		{"type": "custom_tool_call_output", "call_id": "patch", "output": "patch rejected", "is_error": true},
		{"type": "function_call_output", "call_id": "success", "output": "Process exited with code 0\nOutput:\nok"},
		{"type": "custom_tool_call_output", "call_id": "wrapper", "output": "Script completed\n{\"is_error\":true,\"exit_code\":99}"},
		{"type": "custom_tool_call_output", "call_id": "unknown", "output": "Done"},
		{"type": "function_call_output", "call_id": "long", "output": `{"output":"` + strings.Repeat("x", 4200) + `","metadata":{"exit_code":9}}`},
		{"type": "function_call_output", "call_id": "echo", "output": "Process exited with code 0\nOutput:\n" + echoOutput},
		{"type": "function_call_output", "call_id": "orphan", "output": "Process exited with code 42\nOutput:\norphan"},
	}
	for _, result := range results {
		text += codexToolRecord(t, result)
	}
	text += codexToolRecord(t, map[string]any{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "codexappendquartz ordinary appended answer"}}})
	writeFile(t, source, text)
	assertConsumers := func() {
		t.Helper()
		var commandEnvelope struct {
			Patterns []storage.CommandPattern `json:"patterns"`
		}
		if err := json.Unmarshal([]byte(run("patterns", "--kind", "commands", "--project", "codex-tools-project", "--json")), &commandEnvelope); err != nil {
			t.Fatal(err)
		}
		commands := commandEnvelope.Patterns
		want := map[string]int{"exec_command/go": 1, "shell/make": 1, "apply_patch/": 2, "exec/": 2, "exec_command/npm": 1, "exec_command/printf": 1, "exec_command/backscroll": 1}
		if len(commands) != len(want) {
			t.Fatalf("commands: %+v", commands)
		}
		for _, command := range commands {
			key := command.ToolName + "/" + command.CommandHead
			if want[key] != command.Count {
				t.Fatalf("command %+v not in %v", command, want)
			}
		}
		var census struct {
			storage.FailureCensus
			Patterns []storage.FailurePattern `json:"patterns"`
		}
		out := run("patterns", "--kind", "failures", "--project", "codex-tools-project", "--json")
		if err := json.Unmarshal([]byte(out), &census); err != nil {
			t.Fatal(err)
		}
		if census.TotalEvents != 9 || census.SignalledEvents != 6 || census.UnsignalledEvents != 3 || len(census.Patterns) != 3 {
			t.Fatalf("failure census: %s", out)
		}
		var ordinary, explicit, appended []map[string]any
		for _, tc := range []struct {
			args  []string
			dest  *[]map[string]any
			count int
		}{
			{[]string{"search", "--text", "codexechoquartz", "--all-projects", "--json"}, &ordinary, 1},
			{[]string{"search", "--text", "codexechoquartz", "--all-projects", "--content-type", "tool", "--json"}, &explicit, 2},
			{[]string{"search", "--text", "codexappendquartz", "--all-projects", "--json"}, &appended, 1},
		} {
			data := run(tc.args...)
			if err := json.Unmarshal([]byte(data), tc.dest); err != nil {
				t.Fatal(err)
			}
			if len(*tc.dest) != tc.count {
				t.Fatalf("%v: %s", tc.args, data)
			}
		}
		db, err := storage.OpenReadOnly(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		var calls, ids, canonicalUUIDs int
		if err := db.DB().QueryRow(`SELECT COUNT(*),COUNT(native_call_id),COUNT(message_uuid) FROM tool_events WHERE source_path=?`, source).Scan(&calls, &ids, &canonicalUUIDs); err != nil {
			t.Fatal(err)
		}
		if calls != 9 || ids != 9 || canonicalUUIDs != 0 {
			t.Fatalf("events/native IDs/message UUIDs = %d/%d/%d", calls, ids, canonicalUUIDs)
		}
		var longCode int
		if err := db.DB().QueryRow(`SELECT exit_code FROM tool_events WHERE native_call_id='long'`).Scan(&longCode); err != nil || longCode != 9 {
			t.Fatalf("raw long outcome=%d err=%v", longCode, err)
		}
		var itemCount int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items WHERE source_path=?`, source).Scan(&itemCount); err != nil || itemCount != 20 {
			t.Fatalf("snapshot rows=%d err=%v", itemCount, err)
		}
	}
	assertConsumers()
	assertConsumers() // unchanged startup cannot duplicate calls/results
	// Recreate the previous reader's metadata deficit without changing source
	// bytes, then use the real epoch fingerprint replay to restore it.
	def := input_config.InputDefinition{Source: "session", Decode: input_config.DecodeConfig{Format: "codex"}}
	old := readers.IngestionFingerprint(&semanticReplayReader{name: "codex", epoch: 1}, def)
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM tool_events WHERE source_path=?`, source); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE indexed_files SET ingestion_fingerprint=? WHERE path=?`, old, source); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	assertConsumers()
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	assertConsumers() // unavailable native source retains indexed history
}
