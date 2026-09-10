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

// Issue #64 residual cases from the reconciliation scout: direct Backscroll
// searches run from Codex and OpenCode sessions. The historical prose fixture is
// the same Claude corpus as the other query-echo E2E tests; the echoes are
// ingested through the real Codex and OpenCode readers with actual captured CLI
// robot output as the paired result, never a hand-authored imitation.

func TestCodexDirectSearchEchoesDoNotCrowdUnfilteredRecall(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(id, command string) map[string]any
	}{
		{"exec_command", func(id, command string) map[string]any {
			args, _ := json.Marshal(map[string]any{"cmd": command})
			return map[string]any{"type": "function_call", "name": "exec_command", "call_id": id, "arguments": string(args)}
		}},
		{"shell argv", func(id, command string) map[string]any {
			args, _ := json.Marshal(map[string]any{"command": []string{"bash", "-lc", command}})
			return map[string]any{"type": "function_call", "name": "shell", "call_id": id, "arguments": string(args)}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newQueryEchoE2E(t)
			e.writeCoreProse()
			baseline := e.searchJSON(queryEchoText, "", 20)
			if queryEchoRank(baseline, "target.jsonl") != 4 {
				t.Fatalf("unexpected baseline: %v", queryEchoShape(baseline))
			}
			output := e.run("search", "--text", queryEchoText, "--all-projects", "--robot", "--fields", "minimal", "--max-tokens", "0", "--limit", "2")
			codexRoot := e.addReaderManifest("codex", "codex")
			for i := 0; i < 3; i++ {
				id := fmt.Sprintf("codex-%d", i)
				writeCodexRollout(t, filepath.Join(codexRoot, id+".jsonl"), id, 4+i,
					tc.call(id, "backscroll search --text '"+queryEchoText+"' --all-projects --robot --fields minimal --max-tokens 0 --limit 2"),
					map[string]any{"type": "function_call_output", "call_id": id, "output": output})
			}
			e.assertReaderEchoesExcluded(baseline, 6)
		})
	}
}

func TestOpenCodeDirectSearchEchoesDoNotCrowdUnfilteredRecall(t *testing.T) {
	e := newQueryEchoE2E(t)
	e.writeCoreProse()
	baseline := e.searchJSON(queryEchoText, "", 20)
	if queryEchoRank(baseline, "target.jsonl") != 4 {
		t.Fatalf("unexpected baseline: %v", queryEchoShape(baseline))
	}
	output := e.run("search", "--text", queryEchoText, "--all-projects", "--robot", "--fields", "minimal", "--max-tokens", "0", "--limit", "2")
	opencodeRoot := e.addReaderManifest("opencode", "opencode")
	db := openOpenCodeFixture(t, filepath.Join(opencodeRoot, "opencode.db"))
	defer func() { _ = db.Close() }()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("opencode-%d", i)
		insertOpenCodeToolPart(t, db, id, int64(1767225600000+(4+i)*60000), map[string]any{
			"type": "tool", "tool": "bash",
			"state": map[string]any{
				"input":  map[string]any{"command": "backscroll search --text '" + queryEchoText + "' --all-projects --robot --fields minimal --max-tokens 0 --limit 2", "description": "recall"},
				"output": output,
			},
		})
	}
	e.assertReaderEchoesExcluded(baseline, 6)
}

// Negative controls: the same readers must keep unrelated commands with
// identical output, and Codex shell argv that is not the exact direct form.
func TestCodexAndOpenCodeEchoNegativeControlsRemainSearchable(t *testing.T) {
	e := newQueryEchoE2E(t)
	query := "unrelated tool compass"
	codexRoot := e.addReaderManifest("codex", "codex")
	rgArgs, _ := json.Marshal(map[string]any{"cmd": "rg '" + query + "' /synthetic"})
	writeCodexRollout(t, filepath.Join(codexRoot, "rg.jsonl"), "rg", 1,
		map[string]any{"type": "function_call", "name": "exec_command", "call_id": "rg", "arguments": string(rgArgs)},
		map[string]any{"type": "function_call_output", "call_id": "rg", "output": "result_0_snippet=" + query})
	wrapperArgs, _ := json.Marshal(map[string]any{"command": []string{"bash", "-lc", "env backscroll search --text '" + query + "'"}})
	writeCodexRollout(t, filepath.Join(codexRoot, "wrapper.jsonl"), "wrapper", 2,
		map[string]any{"type": "function_call", "name": "shell", "call_id": "wrapper", "arguments": string(wrapperArgs)},
		map[string]any{"type": "function_call_output", "call_id": "wrapper", "output": "result_0_snippet=" + query})
	opencodeRoot := e.addReaderManifest("opencode", "opencode")
	db := openOpenCodeFixture(t, filepath.Join(opencodeRoot, "opencode.db"))
	defer func() { _ = db.Close() }()
	insertOpenCodeToolPart(t, db, "rg", 1767225600000, map[string]any{
		"type": "tool", "tool": "bash",
		"state": map[string]any{"input": map[string]any{"command": "rg '" + query + "' /synthetic"}, "output": "result_0_snippet=" + query},
	})
	e.run("status", "--json")

	unfiltered := e.searchJSON(query, "", 20)
	tool := e.searchJSON(query, "tool", 20)
	if len(unfiltered) != 6 || len(tool) != 6 {
		t.Fatalf("negative controls changed: unfiltered=%v tool=%v", queryEchoShape(unfiltered), queryEchoShape(tool))
	}
}

func (e *queryEchoE2E) assertReaderEchoesExcluded(baseline []queryEchoResult, wantToolRows int) {
	e.t.Helper()
	e.run("status", "--json")
	got := e.searchJSON(queryEchoText, "", 20)
	e.t.Logf("baseline=%v with-echoes=%v", queryEchoShape(baseline), queryEchoShape(got))
	if !reflect.DeepEqual(queryEchoShape(got), queryEchoShape(baseline)) {
		e.t.Errorf("reader echoes changed unfiltered baseline; target rank=%d", queryEchoRank(got, "target.jsonl"))
	}
	if top := e.searchJSON(queryEchoText, "", 5); queryEchoRank(top, "target.jsonl") == 0 {
		e.t.Errorf("reader echoes crowded target out of top five: %v", queryEchoShape(top))
	}
	if tool := e.searchJSON(queryEchoText, "tool", 20); len(tool) != wantToolRows {
		e.t.Errorf("explicit tool search returned %d rows, want %d call/result rows", len(tool), wantToolRows)
	}
	if text := e.searchJSON(queryEchoText, "text", 20); !reflect.DeepEqual(queryEchoShape(text), queryEchoShape(baseline)) {
		e.t.Errorf("text-only recall changed: %v", queryEchoShape(text))
	}
	if repeat := e.searchJSON(queryEchoText, "", 20); !reflect.DeepEqual(repeat, got) {
		e.t.Errorf("repeat changed: %v", queryEchoShape(repeat))
	}
	e.assertIntegrity()
}

// addReaderManifest registers a second active input of the given format rooted
// outside the Claude fixture directory, so the Claude reader never sees it.
func (e *queryEchoE2E) addReaderManifest(id, format string) string {
	e.t.Helper()
	root := filepath.Join(e.root, id+"-fixtures")
	if err := os.MkdirAll(root, 0o700); err != nil {
		e.t.Fatal(err)
	}
	include := "**/*.jsonl"
	if format == "opencode" {
		include = "opencode.db"
	}
	manifest := fmt.Sprintf("version = 1\n[[inputs]]\nid = %q\nsource = %q\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [%q]\nexclude = []\n[inputs.decode]\nformat = %q\n", id+"-echo-e2e", "session", root, include, format)
	if err := os.WriteFile(filepath.Join(e.config, "backscroll", "inputs", id+".inputs.toml"), []byte(manifest), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return root
}

func writeCodexRollout(t *testing.T, path, session string, minute int, items ...map[string]any) {
	t.Helper()
	var lines []string
	line := func(second int, typ string, payload map[string]any) {
		data, err := json.Marshal(map[string]any{"timestamp": fmt.Sprintf("2026-01-01T00:%02d:%02dZ", minute, second), "type": typ, "payload": payload})
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, string(data))
	}
	line(0, "session_meta", map[string]any{"id": session, "cwd": "/synthetic/query-echo-e2e"})
	for i, item := range items {
		line(1+i, "response_item", item)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func openOpenCodeFixture(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertOpenCodeToolPart(t *testing.T, db *sql.DB, id string, ts int64, part map[string]any) {
	t.Helper()
	data, err := json.Marshal(part)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO message VALUES (?, ?, ?, ?, ?)`, id, "active", ts, ts, `{"role":"assistant"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO part VALUES (?, ?, ?, ?, ?, ?)`, "p-"+id, id, "active", ts, ts, string(data)); err != nil {
		t.Fatal(err)
	}
}
