package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// One index must retain native ownership even when providers reuse call IDs.
// Recovery carries canonical records, then available inputs rederive native tools.
func TestMixedProviderNativeOwnershipReplayAndRecovery(t *testing.T) {
	e := newQueryEchoE2E(t)
	write := func(path, data string, appendFile bool) {
		t.Helper()
		flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
		if appendFile {
			flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
		}
		f, err := os.OpenFile(path, flags, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString(data); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	ompRoot := e.addReaderManifest("mixed-omp", "omp")
	omp := filepath.Join(ompRoot, "session.jsonl")
	write(omp, `{"type":"session","version":3,"id":"omp-mixed","cwd":"/synthetic/omp-project","timestamp":"2026-09-01T12:00:00Z"}
{"type":"message","id":"u","timestamp":"2026-09-01T12:00:01Z","message":{"role":"user","attribution":"user","content":"mixedproviderquartz omp"}}
{"type":"message","id":"a","timestamp":"2026-09-01T12:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"answer"},{"type":"thinking","thinking":"mixedreasonquartz"},{"type":"toolCall","id":"shared","name":"bash","arguments":{"command":"echo omp"}}]}}
`, false)
	piRoot := e.addReaderManifest("mixed-pi", "pi")
	pi := filepath.Join(piRoot, "session.jsonl")
	write(pi, `{"type":"session","id":"pi-mixed","cwd":"/synthetic/pi-project"}
{"type":"message","id":"u","timestamp":"2026-09-01T12:00:01Z","message":{"role":"user","content":"mixedproviderquartz pi"}}
{"type":"message","id":"a","timestamp":"2026-09-01T12:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"shared","name":"bash","arguments":{"command":"echo pi"}}]}}
{"type":"message","id":"r","timestamp":"2026-09-01T12:00:03Z","message":{"role":"toolResult","toolCallId":"shared","isError":false,"content":[{"type":"text","text":"ok"}]}}
`, false)
	codexRoot := e.addReaderManifest("mixed-codex", "codex")
	codex := filepath.Join(codexRoot, "rollout.jsonl")
	writeCodexRollout(t, codex, "codex-mixed", 0,
		map[string]any{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "mixedproviderquartz codex"}}},
		map[string]any{"type": "function_call", "call_id": "shared", "name": "exec_command", "arguments": `{"cmd":"false"}`},
		map[string]any{"type": "function_call_output", "call_id": "shared", "output": "Process exited with code 7\nOutput:\nfailed"})
	ocRoot := e.addReaderManifest("mixed-opencode", "opencode")
	oc := filepath.Join(ocRoot, "opencode.db")
	native := openOpenCodeFixture(t, oc)
	if _, err := native.Exec(`CREATE TABLE session(id TEXT PRIMARY KEY,directory TEXT); INSERT INTO session VALUES('s0','/synthetic/oc0'),('s1','/synthetic/oc1')`); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		id, sid := fmt.Sprintf("m%d", i), fmt.Sprintf("s%d", i)
		if _, err := native.Exec(`INSERT INTO message VALUES(?,?,?,?,?)`, id, sid, 1767225600000, 1767225600000, `{"role":"assistant"}`); err != nil {
			t.Fatal(err)
		}
		command := "go test"
		if i == 1 {
			command = "git status"
		}
		for j, part := range []map[string]any{{"type": "text", "text": "mixedproviderquartz " + sid}, {"type": "tool", "tool": "Bash", "state": map[string]any{"input": map[string]any{"command": command}, "output": "done"}}} {
			data, err := json.Marshal(part)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := native.Exec(`INSERT INTO part VALUES(?,?,?,?,?,?)`, fmt.Sprintf("p%d-%d", i, j), id, sid, 1767225600000, 1767225600000, string(data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := native.Close(); err != nil {
		t.Fatal(err)
	}
	query := func(index, statement string, args ...any) [][]string {
		t.Helper()
		db, err := sql.Open("sqlite", "file:"+index+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		rows, err := db.Query(statement, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var result [][]string
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		for rows.Next() {
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			line := make([]string, len(values))
			for i, v := range values {
				line[i] = fmt.Sprint(v)
			}
			result = append(result, line)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	equal := func(got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v; want %#v", got, want)
		}
	}
	equal(len(e.searchJSON("mixedproviderquartz", "", 20)), 5)
	var listing struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(e.run("list", "--all-projects", "--json")), &listing); err != nil {
		t.Fatal(err)
	}
	equal(listing.Count, 5)
	const snapshot = `SELECT COALESCE(uuid,''),ordinal,text,session_id,COALESCE(project,''),origin FROM search_items ORDER BY source_path,ordinal`
	stable := query(e.database, snapshot)
	e.run("status", "--json")
	equal(query(e.database, snapshot), stable)
	equal(len(e.searchJSON("mixedreasonquartz", "reasoning", 20)), 0)
	coords := query(e.database, `SELECT id,uuid,ordinal FROM search_items WHERE uuid LIKE 'omp:%' ORDER BY uuid`)
	manifestPath := filepath.Join(e.config, "backscroll", "inputs", "mixed-omp.inputs.toml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	write(manifestPath, string(manifest)+"index_reasoning = true\n", false)
	equal(len(e.searchJSON("mixedreasonquartz", "reasoning", 20)), 1)
	after := query(e.database, `SELECT id,uuid,ordinal FROM search_items WHERE uuid LIKE 'omp:%' AND content_type <> 'reasoning' ORDER BY uuid`)
	equal(after, coords)
	write(omp, `{"type":"message","id":"r","timestamp":"2026-09-01T12:00:03Z","message":{"role":"toolResult","toolCallId":"shared","toolName":"bash","isError":true,"details":{"exitCode":9,"__interrupted":true},"content":[{"type":"text","text":"failed"}]}}
`, true)
	e.run("status", "--json")
	const outcomes = `SELECT native_call_id,COALESCE(is_error,-1),COALESCE(exit_code,-1),COALESCE(message_uuid,'') FROM tool_events WHERE native_call_id IS NOT NULL ORDER BY source_path,native_call_id`
	equal(query(e.database, outcomes), [][]string{{"shared", "1", "7", ""}, {"shared", "1", "9", "omp:omp-mixed:a#b2"}, {"shared", "0", "-1", "pi:pi-mixed:call:shared"}})
	equal(query(e.database, `SELECT was_interrupted FROM search_items WHERE uuid='omp:omp-mixed:a#b2'`), [][]string{{"1"}})
	for i := range 2 {
		var found []struct {
			UUID    string `json:"uuid"`
			Session string `json:"session_id"`
		}
		if err := json.Unmarshal([]byte(e.run("search", "--text", "mixedproviderquartz", "--project", fmt.Sprintf("oc%d", i), "--fields", "minimal", "--json")), &found); err != nil {
			t.Fatal(err)
		}
		equal(len(found), 1)
		equal(found[0].Session, fmt.Sprintf("s%d", i))
		var context struct {
			Records []struct {
				Session string `json:"session_id"`
				Project string `json:"project"`
			} `json:"records"`
		}
		if err := json.Unmarshal([]byte(e.run("context", "--uuid", found[0].UUID, "--before", "50", "--after", "50", "--json")), &context); err != nil {
			t.Fatal(err)
		}
		equal(len(context.Records), 3)
		for _, r := range context.Records {
			equal(r.Session, found[0].Session)
			equal(r.Project, fmt.Sprintf("oc%d", i))
		}
	}
	stable = query(e.database, snapshot)
	signals := query(e.database, outcomes)
	e.run("rebuild")
	equal(query(e.database, snapshot), stable)
	equal(query(e.database, outcomes), signals)
	available := newQueryEchoE2E(t)
	available.run("status", "--json")
	available.run("recover", "--from", e.database)
	equal(query(available.database, snapshot), stable)
	// Replay through the original available source capabilities restores native IDs.
	for _, name := range []string{"mixed-omp", "mixed-pi", "mixed-codex", "mixed-opencode"} {
		data, err := os.ReadFile(filepath.Join(e.config, "backscroll", "inputs", name+".inputs.toml"))
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(available.config, "backscroll", "inputs", name+".inputs.toml"), string(data), false)
	}
	available.run("rebuild")
	equal(query(available.database, snapshot), stable)
	equal(query(available.database, outcomes), signals)
	for _, path := range []string{omp, pi, codex, oc} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	expired := newQueryEchoE2E(t)
	expired.run("status", "--json")
	expired.run("recover", "--from", e.database)
	equal(query(expired.database, snapshot), stable)
	expired.run("rebuild")
	equal(query(expired.database, `SELECT COUNT(*),COUNT(native_call_id),COUNT(message_uuid) FROM tool_events WHERE source_path=?`, omp), [][]string{{"1", "0", "0"}})
	equal(query(expired.database, `SELECT session_id,COUNT(*) FROM search_items WHERE session_id <> '' GROUP BY session_id ORDER BY session_id`), [][]string{{"s0", "3"}, {"s1", "3"}})
	if !strings.Contains(expired.run("search", "--text", "mixedproviderquartz", "--tag", "testing", "--all-projects", "--robot"), "session_id=s0") {
		t.Fatal("recovered testing tag must retain owning OpenCode session")
	}
}
