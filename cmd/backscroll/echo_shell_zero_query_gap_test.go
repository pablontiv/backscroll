package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/startuplock"
)

// Spike/regression for the Codex `shell` wrapper form of the zero-valued
// search_echo query-time gap. The exec_command half of this gap was fixed in
// PR #86; the requeue side learned the shell form in PR #87, but
// isDirectBackscrollSearchEcho / directBackscrollSearchEchoSQL (the functions
// that exclude a row from unfiltered result pages and --relax IDF counting
// RIGHT NOW, before reparse converges it) never gained shell handling.
//
// The fixture is a real CodexReader.Parse round-trip: the rollout below is
// ingested by the actual Codex reader, its serialized shape is asserted, and
// only then is search_echo forced back to 0 to reproduce the pre-#80 state.
func TestZeroValuedCodexShellEchoExcludedBeforeReplay(t *testing.T) {
	e := newQueryEchoE2E(t)
	e.writeRecord("target.jsonl", "target", "target", 0, "violet handshake quartz marker", false)
	for i := 0; i < 4; i++ {
		e.writeRecord(fmt.Sprintf("noise-%d.jsonl", i), fmt.Sprintf("noise-%d", i), "noise", 1+i, "adaptation rollout distractor", false)
	}

	codexRoot := e.addReaderManifest("codex", "codex")
	// Real Codex shell calls carry extra keys (workdir, timeout_ms), so the
	// serialized `command=` token is not the only key=value and the strict
	// predicate must locate it inside the sorted token list.
	for i := 0; i < 8; i++ {
		id := fmt.Sprintf("codex-shell-%d", i)
		args, _ := json.Marshal(map[string]any{
			"command":    []string{"bash", "-lc", "backscroll search --text 'violet handshake'"},
			"workdir":    "/synthetic/query-echo-e2e",
			"timeout_ms": 10000,
		})
		writeCodexRollout(t, filepath.Join(codexRoot, id+".jsonl"), id, 10+i,
			map[string]any{"type": "function_call", "name": "shell", "call_id": id, "arguments": string(args)})
	}
	// Already-fixed exec_command control in the same forced-zero state: it
	// must stay excluded by both the Go and the SQL/IDF paths.
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("codex-exec-%d", i)
		args, _ := json.Marshal(map[string]any{"cmd": "backscroll search --text 'violet handshake'"})
		writeCodexRollout(t, filepath.Join(codexRoot, id+".jsonl"), id, 20+i,
			map[string]any{"type": "function_call", "name": "exec_command", "call_id": id, "arguments": string(args)})
	}
	e.run("status", "--json")

	db, err := sql.Open("sqlite", e.database)
	if err != nil {
		t.Fatal(err)
	}
	var serialized string
	if err := db.QueryRow(`SELECT text FROM search_items WHERE source_path LIKE ? AND content_type='tool' LIMIT 1`, "%codex-shell-0.jsonl").Scan(&serialized); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	const wantSerialized = `shell command=["bash","-lc","backscroll search --text 'violet handshake'"] timeout_ms=10000 workdir=/synthetic/query-echo-e2e`
	if serialized != wantSerialized {
		_ = db.Close()
		t.Fatalf("Codex shell SerializeToolInput shape=%q want %q", serialized, wantSerialized)
	}
	if _, err := db.Exec(`UPDATE search_items SET search_echo=0 WHERE content_type='tool' AND source_path LIKE ?`, "%codex-%.jsonl"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Hold the startup lock so the following searches run as followers on the
	// last committed snapshot: the query-time fallback is what must exclude the
	// zeroed rows, with no owner replay to converge them first.
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
	for _, row := range got {
		if row.ContentType == "tool" && strings.HasPrefix(filepath.Base(row.FilePath), "codex-") {
			t.Errorf("zero-valued Codex echo leaked into unfiltered recall: %v", queryEchoShape(got))
		}
	}

	out := e.run("search", "--text", "violet handshake adaptation", "--all-projects", "--lexical-only", "--relax", "--robot", "--fields", "minimal", "--max-tokens", "200")
	if !strings.Contains(out, "result_0_filepath="+filepath.Join(e.fixtures, "target.jsonl")+"\n") || !strings.Contains(out, `result_0_dropped_terms=["adaptation"]`) {
		t.Errorf("zero-valued shell query echoes inverted unfiltered --relax IDF\nstdout=%s", out)
	}
}
