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
	"time"

	"github.com/pablontiv/backscroll/internal/startuplock"
)

// A startup-lock follower reads the committed snapshot without replaying the
// bounded search_echo backlog. That makes the query-time fallback observable
// while a surviving source is still waiting for its owner sync.
func TestZeroValuedCodexExecCommandEchoExcludedBeforeReplay(t *testing.T) {
	e := newQueryEchoE2E(t)
	e.writeRecord("target.jsonl", "target", "target", 0, "violet handshake quartz marker", false)
	for i := 0; i < 4; i++ {
		e.writeRecord(fmt.Sprintf("noise-%d.jsonl", i), fmt.Sprintf("noise-%d", i), "noise", 1+i, "adaptation rollout distractor", false)
	}
	e.writeRecord("claude-echo.jsonl", "claude-echo", "claude-echo", 6,
		queryEchoToolBlock("claude-echo", "backscroll search --text 'violet handshake'"), false)
	e.run("status", "--json")

	setZero := func(pattern string) {
		t.Helper()
		db, err := sql.Open("sqlite", e.database)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = db.Close() }()
		if _, err := db.Exec(`UPDATE search_items SET search_echo=0 WHERE content_type='tool' AND source_path LIKE ?`, pattern); err != nil {
			t.Fatal(err)
		}
	}
	assertFollower := func(wantCodexLeaks int) {
		t.Helper()
		lease, acquired, err := startuplock.TryAcquire(e.database)
		if err != nil || !acquired {
			t.Fatalf("acquire parent startup lock: acquired=%v err=%v", acquired, err)
		}
		defer func() {
			if err := lease.Release(); err != nil {
				t.Errorf("release parent startup lock: %v", err)
			}
		}()

		got := e.searchJSON("violet handshake", "", 20)
		var codexLeaks int
		for _, row := range got {
			if row.ContentType == "tool" && strings.HasPrefix(filepath.Base(row.FilePath), "codex-") {
				codexLeaks++
			}
			if filepath.Base(row.FilePath) == "claude-echo.jsonl" {
				t.Errorf("zero-valued Claude fallback echo leaked: %v", queryEchoShape(got))
			}
		}
		if codexLeaks != wantCodexLeaks {
			t.Errorf("zero-valued Codex fallback leaks=%d want %d: %v", codexLeaks, wantCodexLeaks, queryEchoShape(got))
		}

		out := e.run("search", "--text", "violet handshake adaptation", "--all-projects", "--lexical-only", "--relax", "--robot", "--fields", "minimal", "--max-tokens", "200")
		if !strings.Contains(out, "result_0_filepath="+filepath.Join(e.fixtures, "target.jsonl")+"\n") || !strings.Contains(out, `result_0_dropped_terms=["adaptation"]`) {
			t.Errorf("zero-valued query echoes changed unfiltered --relax IDF\nstdout=%s", out)
		}
	}

	setZero("%claude-echo.jsonl")
	assertFollower(0)

	codexRoot := e.addReaderManifest("codex", "codex")
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("codex-%d", i)
		args, _ := json.Marshal(map[string]any{
			"cmd":     "backscroll search --text 'violet handshake'",
			"command": [][]string{{"unused"}},
		})
		writeCodexRollout(t, filepath.Join(codexRoot, id+".jsonl"), id, 10+i,
			map[string]any{"type": "function_call", "name": "exec_command", "call_id": id, "arguments": string(args)})
	}
	e.run("status", "--json")

	db, err := sql.Open("sqlite", e.database)
	if err != nil {
		t.Fatal(err)
	}
	var serialized string
	if err := db.QueryRow(`SELECT text FROM search_items WHERE source_path LIKE ? LIMIT 1`, "%codex-0.jsonl").Scan(&serialized); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	const wantSerialized = `exec_command cmd=backscroll search --text 'violet handshake' command=[["unused"]]`
	if serialized != wantSerialized {
		t.Fatalf("Codex SerializeToolInput shape=%q want %q", serialized, wantSerialized)
	}

	setZero("%codex-%.jsonl")
	assertFollower(0)
}

// Pre-#80 Codex writes stored search_echo=0 (not NULL). Hash-stable reopen must
// still reparse those surviving sources so call_id pairing can mark results.
func TestStaleZeroSearchEchoReplaysOnHashStableReopen(t *testing.T) {
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
		args, _ := json.Marshal(map[string]any{"cmd": "backscroll search --text '" + queryEchoText + "' --all-projects --robot --fields minimal --max-tokens 0 --limit 2"})
		writeCodexRollout(t, filepath.Join(codexRoot, id+".jsonl"), id, 4+i,
			map[string]any{"type": "function_call", "name": "exec_command", "call_id": id, "arguments": string(args)},
			map[string]any{"type": "function_call_output", "call_id": id, "output": output})
	}
	e.run("status", "--json")
	if got := queryEchoShape(e.searchJSON(queryEchoText, "", 20)); !reflect.DeepEqual(got, queryEchoShape(baseline)) {
		t.Fatalf("fresh Codex ingest should already exclude echoes: %v", got)
	}

	db, err := sql.Open("sqlite", e.database)
	if err != nil {
		t.Fatal(err)
	}
	var marked, pending int
	if err := db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE search_echo=1`).Scan(&marked); err != nil {
		t.Fatal(err)
	}
	if marked == 0 {
		t.Fatal("expected current Codex reader to persist search_echo=1")
	}
	if _, err := db.Exec(`UPDATE search_items SET search_echo=0 WHERE content_type='tool'`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM search_items WHERE search_echo IS NULL`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("NULL backlog should stay empty after forcing 0, got %d", pending)
	}
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries, err := os.ReadDir(codexRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := filepath.Join(codexRoot, entry.Name())
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE indexed_files SET last_indexed=?, file_size=?, file_mtime=? WHERE path=?`,
			"2026-06-01 00:00:00", info.Size(), old.Format(time.RFC3339), path); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	e.run("status", "--json")
	got := e.searchJSON(queryEchoText, "", 20)
	top := e.searchJSON(queryEchoText, "", 5)
	if !reflect.DeepEqual(queryEchoShape(got), queryEchoShape(baseline)) {
		t.Errorf("hash-stable reopen left zeroed Codex echoes in unfiltered recall; target rank=%d\ngot:  %v\nwant: %v", queryEchoRank(got, "target.jsonl"), queryEchoShape(got), queryEchoShape(baseline))
	}
	if queryEchoRank(top, "target.jsonl") == 0 {
		t.Errorf("target crowded out of top five: %v", queryEchoShape(top))
	}
}
