package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Exercise the real command's output, not a hand-authored imitation of robot text.
func TestPairedBackscrollSearchResultsDoNotCrowdRecall(t *testing.T) {
	e := newQueryEchoE2E(t)
	paths := e.writeCoreProse()
	baseline := e.searchJSON(queryEchoText, "", 20)
	if queryEchoRank(baseline, "target.jsonl") != 4 {
		t.Fatalf("unexpected baseline: %v", queryEchoShape(baseline))
	}
	output := e.run("search", "--text", queryEchoText, "--all-projects", "--robot", "--fields", "minimal", "--max-tokens", "0", "--limit", "2")
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("paired-%d", i)
		path := e.writeRecord(id+".jsonl", id, "active", 4+i, queryEchoToolBlock(id, "backscroll search --text '"+queryEchoText+"' --all-projects --robot --fields minimal --max-tokens 0 --limit 2"), false)
		e.appendToolResult(path, id+"-result", id, output)
		paths = append(paths, path)
	}
	assertRecall := func() {
		t.Helper()
		got := e.searchJSON(queryEchoText, "", 20)
		t.Logf("baseline=%v paired=%v", queryEchoShape(baseline), queryEchoShape(got))
		if !reflect.DeepEqual(queryEchoShape(got), queryEchoShape(baseline)) {
			t.Errorf("paired results changed baseline; target rank=%d", queryEchoRank(got, "target.jsonl"))
		}
		if got := e.searchJSON(queryEchoText, "", 5); queryEchoRank(got, "target.jsonl") == 0 {
			t.Errorf("paired results crowded target out of top five: %v", queryEchoShape(got))
		}
		if got := e.searchJSON(queryEchoText, "tool", 20); len(got) != 6 {
			t.Errorf("explicit tool search returned %d rows, want all six command/result rows", len(got))
		}
		if got := e.searchJSON(queryEchoText, "text", 20); !reflect.DeepEqual(queryEchoShape(got), queryEchoShape(baseline)) {
			t.Errorf("text-only recall changed: %v", queryEchoShape(got))
		}
	}
	assertRecall()
	// Persistence must outlive source expiry without deleting the original rows.
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	assertRecall()
	if got := e.countCoreRows(paths); got != 10 {
		t.Errorf("stored rows = %d, want 10", got)
	}
	e.assertIntegrity()
}

func (e *queryEchoE2E) appendToolResult(path, uuid, callID, output string) {
	e.t.Helper()
	record := map[string]any{
		"uuid": uuid, "type": "user", "timestamp": "2026-01-01T00:09:00Z", "sessionId": "active", "cwd": "/synthetic/query-echo-e2e",
		"message": map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": callID, "content": output, "is_error": false}}},
	}
	data, err := json.Marshal(record)
	if err != nil {
		e.t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		e.t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		e.t.Fatal(err)
	}
}
