package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/readers"
)

// shellText serializes a Codex shell argv triple via a real json.Marshal
// round-trip into readers.SerializeToolInput, so the stored text mirrors what
// the CodexReader actually persists for an accepted argv shape. The earlier
// fmt.Sprintf+%q fixture used Go's strconv.Quote escaping, which escapes
// NBSP (U+00A0) as `\u00a0` text — but real JSON encoders (Go's encoding/json
// and Rust serde_json) only escape control chars (<0x20) and emit other
// unicode.IsSpace runes as their raw UTF-8 bytes. Building fixtures through
// a real json.Marshal round-trip catches encoding-divergence bugs the string
// quoting cannot.
func shellText(t *testing.T, shellBin, flag, argv2 string) string {
	t.Helper()
	args := struct {
		Command []string `json:"command"`
	}{Command: []string{shellBin, flag, argv2}}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return readers.SerializeToolInput("shell", raw)
}

// shellTextN serializes a Codex shell call with an arbitrary argv length. Used
// for 4+ element fixtures the reader rejects via len(Command)==3, whose
// over-match guard must still hold for every JSON-escape separator form.
func shellTextN(t *testing.T, shellBin, flag string, argvRest ...string) string {
	t.Helper()
	argv := append([]string{shellBin, flag}, argvRest...)
	args := struct {
		Command []string `json:"command"`
	}{Command: argv}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return readers.SerializeToolInput("shell", raw)
}

func TestV15EchoBackfillPreservesPerennialIdentity(t *testing.T) {
	path := createFixtureDatabase(t, "v14.sql")
	db, err := openWithoutSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct{ path, uuid, text, kind string }{
		{"live.jsonl", "result", "orchard result", "tool"},
		{"expired.jsonl", "expired", "orchard result", "tool"},
		{"prose.jsonl", "prose", "orchard decision", "text"},
	} {
		exec(`INSERT INTO search_items(source,source_path,ordinal,role,text,uuid,content_type,extraction_version) VALUES('session',?,0,'user',?,?,?,?)`, row.path, row.text, row.uuid, row.kind, CurrentExtractionVersion)
	}
	var originalID int
	if err := db.db.QueryRow(`SELECT id FROM search_items WHERE uuid='result'`).Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	plan, diag, err := compat.InspectIndex(context.Background(), db.db)
	if err != nil || diag != nil {
		t.Fatalf("inspect: %v %v", err, diag)
	}
	if err := db.ApplyMigrationPlan(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	paths, err := db.StalePaths(CurrentExtractionVersion)
	if err != nil || len(paths) != 3 {
		t.Fatalf("backlog=%v err=%v", paths, err)
	}
	// An unproven expired output remains visible; no adjacency/output guessing.
	got, err := db.Search("orchard", models.SearchOptions{AllProjects: true})
	if err != nil || len(got) != 3 {
		t.Fatalf("migration lost rows: %v %v", got, err)
	}
	file := IndexedFile{SourcePath: "live.jsonl", Source: "session", Hash: "same", Messages: []IndexedMessage{{Ordinal: 0, Role: "user", Text: "orchard result", UUID: "result", ContentType: "tool", ExtractionVersion: CurrentExtractionVersion, SearchEcho: true}}}
	for i := 0; i < 2; i++ {
		if err := db.SyncFiles([]IndexedFile{file}); err != nil {
			t.Fatal(err)
		}
	}
	var id, echo, version int
	var text string
	if err := db.db.QueryRow(`SELECT id,text,search_echo,extraction_version FROM search_items WHERE uuid='result'`).Scan(&id, &text, &echo, &version); err != nil {
		t.Fatal(err)
	}
	if id != originalID || text != "orchard result" || echo != 1 || version != CurrentExtractionVersion {
		t.Fatalf("identity/provenance changed: %d %q %d %d", id, text, echo, version)
	}
	paths, err = db.StalePaths(CurrentExtractionVersion)
	if err != nil || !reflect.DeepEqual(paths, []string{"expired.jsonl", "prose.jsonl"}) {
		t.Fatalf("backfill failed to converge: %v %v", paths, err)
	}
	got, err = db.Search("orchard", models.SearchOptions{AllProjects: true})
	if err != nil || len(got) != 2 {
		t.Fatalf("unfiltered echo leak: %v %v", got, err)
	}
	got, err = db.Search("orchard", models.SearchOptions{AllProjects: true, ContentType: "tool"})
	if err != nil || len(got) != 2 {
		t.Fatalf("explicit tool lost data: %v %v", got, err)
	}
	// Partial future parses must not erase prior positive pairing evidence.
	file.Messages[0].SearchEcho = false
	if err := db.SyncFiles([]IndexedFile{file}); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT search_echo FROM search_items WHERE uuid='result'`).Scan(&echo); err != nil || echo != 1 {
		t.Fatalf("lost proven pairing: %d %v", echo, err)
	}
	var unknown sql.NullBool
	if err := db.db.QueryRow(`SELECT search_echo FROM search_items WHERE uuid='expired'`).Scan(&unknown); err != nil || unknown.Valid {
		t.Fatalf("expired evidence was guessed: %v %v", unknown, err)
	}
}

func TestPendingSearchEchoPathsRequeuesZeroValuedDirectCalls(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files := []IndexedFile{
		{Source: "session", SourcePath: "codex.jsonl", Hash: "h1", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "exec_command cmd=backscroll search --text orchard", ContentType: "tool", SearchEcho: true},
			{Ordinal: 1, Role: "tool", Text: "result_0_snippet=orchard", ContentType: "tool", SearchEcho: true},
		}},
		{Source: "session", SourcePath: "opencode.db", Hash: "h2", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "bash command=backscroll search --text orchard", ContentType: "tool", SearchEcho: true},
			{Ordinal: 1, Role: "assistant", Text: "result_0_snippet=orchard", ContentType: "tool", SearchEcho: true},
		}},
		{Source: "session", SourcePath: "rg.jsonl", Hash: "h3", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "exec_command cmd=rg orchard", ContentType: "tool"},
		}},
		{Source: "session", SourcePath: "unknown.jsonl", Hash: "h4", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: "result_0_snippet=orchard", ContentType: "tool"},
		}},
	}
	files[3].Messages[0].SearchEcho = false
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo=0 WHERE source_path IN ('codex.jsonl','opencode.db','rg.jsonl')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo=NULL WHERE source_path='unknown.jsonl'`); err != nil {
		t.Fatal(err)
	}
	got, err := db.PendingSearchEchoPaths()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"codex.jsonl", "opencode.db", "unknown.jsonl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pending=%v want %v", got, want)
	}
}

func TestPendingSearchEchoPathsRequeuesZeroValuedCodexShellCall(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Each fixture stores one tool row at search_echo=0 to simulate a pre-#80
	// Codex writer. The requeue expectation mirrors isCodexDirectSearchCall's
	// argv shape: argv is exactly [<shell>, -c|-lc, "backscroll search ..."].
	files := []IndexedFile{
		// Requeued: bare 3-element -c shell direct search.
		{Source: "session", SourcePath: "shell_bare_c.jsonl", Hash: "h1", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["sh","-c","backscroll search"]`, ContentType: "tool"},
		}},
		// Requeued: 3-element -c with extra args.
		{Source: "session", SourcePath: "shell_args_c.jsonl", Hash: "h2", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["sh","-c","backscroll search --text orchard"]`, ContentType: "tool"},
		}},
		// Requeued: 3-element -lc with /bin/bash path.
		{Source: "session", SourcePath: "shell_args_lc.jsonl", Hash: "h3", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["/bin/bash","-lc","backscroll search --text orchard"]`, ContentType: "tool"},
		}},
		// NOT requeued: different command (argv[2]="ls").
		{Source: "session", SourcePath: "shell_ls.jsonl", Hash: "h4", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["sh","-c","ls"]`, ContentType: "tool"},
		}},
		// NOT requeued: wrong flag (argv[1]="-x").
		{Source: "session", SourcePath: "shell_xflag.jsonl", Hash: "h5", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["sh","-x","backscroll search"]`, ContentType: "tool"},
		}},
		// NOT requeued: 4-element argv with trailing 4th element.
		{Source: "session", SourcePath: "shell_four.jsonl", Hash: "h6", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["sh","-c","backscroll search","bar"]`, ContentType: "tool"},
		}},
		// NOT requeued: 4-element argv with trailing whitespace + 4th element (over-match guard).
		{Source: "session", SourcePath: "shell_four_ws.jsonl", Hash: "h7", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["sh","-c","backscroll search ","bar"]`, ContentType: "tool"},
		}},
		// NOT requeued: 4-element argv with -lc flag.
		{Source: "session", SourcePath: "shell_four_lc.jsonl", Hash: "h8", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: `shell command=["bash","-lc","backscroll search","bar"]`, ContentType: "tool"},
		}},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	// Force every row to search_echo=0 so only the new SQL clause can requeue them.
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo=0`); err != nil {
		t.Fatal(err)
	}
	got, err := db.PendingSearchEchoPaths()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"shell_args_c.jsonl", "shell_args_lc.jsonl", "shell_bare_c.jsonl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pending=%v want %v", got, want)
	}
}

// TestPendingSearchEchoPathsRequeuesZeroValuedCodexShellSerializedWhitespace
// walks the reader's strings.Fields acceptance for every unicode.IsSpace
// separator between 'backscroll' and 'search' inside the shell argv's third
// element. Each fixture is built via a real json.Marshal round-trip into
// readers.SerializeToolInput, so the stored text carries whatever the JSON
// encoder actually produces — control chars (U+0009/000A/000D/000C) escape
// to \t/\n/\r/\f, but every other unicode.IsSpace rune (NBSP U+00A0, NEL
// U+0085, en/em spaces U+2002-2003, …) survives as raw UTF-8 bytes that no
// SQL GLOB can keep up with. The replay check must therefore do what the
// reader does — decode the JSON argv and re-run strings.Fields — and this
// test pins that behavior down.
func TestPendingSearchEchoPathsRequeuesZeroValuedCodexShellSerializedWhitespace(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files := []IndexedFile{
		// Requeued: literal space (control case).
		{Source: "session", SourcePath: "sep_space.jsonl", Hash: "h1", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll search orchard"), ContentType: "tool"},
		}},
		// Requeued: JSON \t escape (U+0009 < 0x20 → escapes).
		{Source: "session", SourcePath: "sep_tab.jsonl", Hash: "h2", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll\tsearch orchard"), ContentType: "tool"},
		}},
		// Requeued: JSON \n escape (U+000A < 0x20).
		{Source: "session", SourcePath: "sep_newline.jsonl", Hash: "h3", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll\nsearch orchard"), ContentType: "tool"},
		}},
		// Requeued: JSON \r escape (U+000D < 0x20).
		{Source: "session", SourcePath: "sep_cr.jsonl", Hash: "h4", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll\rsearch orchard"), ContentType: "tool"},
		}},
		// Requeued: JSON \f escape (U+000C < 0x20).
		{Source: "session", SourcePath: "sep_ff.jsonl", Hash: "h5", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll\fsearch orchard"), ContentType: "tool"},
		}},
		// Requeued: NBSP (U+00A0) — raw UTF-8 bytes (0xc2 0xa0), no \uXXXX escape.
		// This is the case the round-2 SQL GLOB missed.
		{Source: "session", SourcePath: "sep_nbsp.jsonl", Hash: "h6", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll\u00a0search orchard"), ContentType: "tool"},
		}},
		// Requeued: en space (U+2002) — raw UTF-8 bytes (0xe2 0x80 0x82).
		{Source: "session", SourcePath: "sep_en_space.jsonl", Hash: "h7", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll\u2002search orchard"), ContentType: "tool"},
		}},
		// Requeued: em space (U+2003) — raw UTF-8 bytes (0xe2 0x80 0x83).
		{Source: "session", SourcePath: "sep_em_space.jsonl", Hash: "h8", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "sh", "-c", "backscroll\u2003search orchard"), ContentType: "tool"},
		}},
		// Requeued: NBSP with -lc flag and /bin/bash path (cross-flag sanity).
		{Source: "session", SourcePath: "sep_nbsp_lc.jsonl", Hash: "h9", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellText(t, "/bin/bash", "-lc", "backscroll\u00a0search orchard"), ContentType: "tool"},
		}},
		// NOT requeued: 4-element argv with NBSP separator + 4th element
		// (over-match guard holds for raw UTF-8 separators too).
		{Source: "session", SourcePath: "sep_nbsp_four.jsonl", Hash: "h10", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellTextN(t, "sh", "-c", "backscroll\u00a0search ", "bar"), ContentType: "tool"},
		}},
		// NOT requeued: 4-element argv with tab separator + 4th element.
		{Source: "session", SourcePath: "sep_tab_four.jsonl", Hash: "h11", Messages: []IndexedMessage{
			{Ordinal: 0, Role: "assistant", Text: shellTextN(t, "sh", "-c", "backscroll\tsearch ", "bar"), ContentType: "tool"},
		}},
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo=0`); err != nil {
		t.Fatal(err)
	}
	got, err := db.PendingSearchEchoPaths()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sep_cr.jsonl", "sep_em_space.jsonl", "sep_en_space.jsonl", "sep_ff.jsonl", "sep_nbsp.jsonl", "sep_nbsp_lc.jsonl", "sep_newline.jsonl", "sep_space.jsonl", "sep_tab.jsonl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pending=%v want %v", got, want)
	}
}

func TestEchoProvenanceDoesNotAffectProse(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SyncFiles([]IndexedFile{{Source: "session", SourcePath: "p", Hash: "h", Messages: []IndexedMessage{{Text: "orchard", UUID: "u", Role: "assistant", ContentType: "text", SearchEcho: true}}}}); err != nil {
		t.Fatal(err)
	}
	got, err := db.Search("orchard", models.SearchOptions{AllProjects: true})
	if err != nil || len(got) != 1 {
		t.Fatalf("prose filtered: %v %v", got, err)
	}
}

// TestPendingSearchEchoPathsRequeuesCodexShellRoundTrip is the regression
// fixture for the round-3 reviewer finding. Real Codex shell calls carry
// not just `command` but also `workdir`, `timeout_ms`, and potentially
// `additional_permissions` — Codex's own ShellToolCallParams schema.
// SerializeToolInput sorts the keys alphabetically, so a call with
// `additional_permissions` serializes as
// `shell additional_permissions={...} command=[...] workdir=/tmp timeout_ms=10000`,
// not `shell command=[...] workdir=/tmp timeout_ms=10000`. The requeue path
// must accept both orderings (Bug B: broaden the admission past the literal
// `shell command=` prefix and locate the `command=` token boundary inside
// the sorted list) and must read only the JSON array that follows,
// ignoring the trailing key=value tokens (Bug A: don't try to wrap the
// entire remainder as a JSON object).
//
// The fixture is generated via a full CodexReader.Parse round-trip of an
// inline rollout JSONL — not via SerializeToolInput directly, not via a
// hand-written string — so it exercises the same storage path the production
// ingest uses.
func TestPendingSearchEchoPathsRequeuesCodexShellRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Three shapes: command-only (control), command + workdir + timeout_ms,
	// and the worst case — additional_permissions sorts before command, so
	// the legacy `text LIKE 'shell command=[%'` prefilter would miss it.
	cases := []struct {
		name, argsJSON string
		requeue       bool
	}{
		{"control", `{"command":["sh","-c","backscroll search --text orchard"]}`, true},
		{"workdir_timeout_ms", `{"command":["sh","-c","backscroll search"],"workdir":"/tmp","timeout_ms":10000}`, true},
		{"additional_permissions_first", `{"additional_permissions":{"network":false},"command":["bash","-lc","backscroll search --text orchard"]}`, true},
		{"workdir_first", `{"workdir":"/tmp","command":["sh","-c","backscroll search"]}`, true},
		{"wrong_command", `{"command":["sh","-c","ls"],"workdir":"/tmp","timeout_ms":10000}`, false},
		{"four_elements", `{"command":["sh","-c","backscroll search","bar"],"workdir":"/tmp"}`, false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jsonl := "{\"ordinal\":0,\"timestamp\":\"2026-09-01T12:00:00Z\",\"type\":\"session_meta\",\"payload\":{\"cwd\":\"/synthetic/test\"}}\n" +
				"{\"ordinal\":1,\"timestamp\":\"2026-09-01T12:00:01Z\",\"type\":\"response_item\",\"payload\":{\"type\":\"function_call\",\"name\":\"shell\",\"call_id\":\"test-call\",\"arguments\":\"" + strings.ReplaceAll(tc.argsJSON, "\"", "\\\"") + "\"}}\n"
			dir := t.TempDir()
			path := filepath.Join(dir, "codex.jsonl")
			if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
				t.Fatal(err)
			}
			parsed, err := (&readers.CodexReader{}).Parse(path, input_config.InputDefinition{})
			if err != nil {
				t.Fatal(err)
			}
			var storedText string
			for _, rec := range parsed.Records {
				if rec.ContentType == "tool" && strings.HasPrefix(rec.Content, "shell ") {
					storedText = rec.Content
					break
				}
			}
			if storedText == "" {
				t.Fatal("shell record not parsed")
			}
			db2, err := Open(filepath.Join(t.TempDir(), "db-"+tc.name+".db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db2.Close()
			sourcePath := "shell_" + tc.name + ".jsonl"
			if err := db2.SyncFiles([]IndexedFile{{Source: "session", SourcePath: sourcePath, Hash: "h", Messages: []IndexedMessage{{Ordinal: 0, Role: "assistant", Text: storedText, ContentType: "tool"}}}}); err != nil {
				t.Fatal(err)
			}
			if _, err := db2.db.Exec(`UPDATE search_items SET search_echo=0`); err != nil {
				t.Fatal(err)
			}
			got, err := db2.PendingSearchEchoPaths()
			if err != nil {
				t.Fatal(err)
			}
			if tc.requeue {
				if !reflect.DeepEqual(got, []string{sourcePath}) {
					t.Errorf("case %d (%s): pending=%v want [%s]", i, tc.name, got, sourcePath)
				}
			} else {
				if len(got) != 0 {
					t.Errorf("case %d (%s): pending=%v want []", i, tc.name, got)
				}
			}
		})
	}
}
