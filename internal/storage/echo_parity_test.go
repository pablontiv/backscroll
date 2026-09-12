package storage

// Three-way echo-exclusion parity: for every serialized direct-search-call
// shape (bash, exec_command, shell) crossed with every separator alphabet
// strings.Fields accepts, the unfiltered page exclusion, the --relax IDF
// exclusion, and the requeue detection must agree. This is the structural
// regression net for the bug class behind #64/#86/#87/#89: previously each
// path recognized echoes with its own hand-written matcher (SQL GLOB with an
// ASCII-only whitespace alphabet vs Go strings.Fields), so any separator the
// SQL alphabet missed — NBSP, U+2028/2029, U+3000 — made pages and IDF
// diverge. There is now exactly one strict predicate
// (directsearch.IsSerializedDirectSearchCall) behind a broad SQL prefilter;
// this test would fail on any reintroduced per-path recognizer.

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/pablontiv/backscroll/internal/readers"
)

// echoSeparators samples the unicode.IsSpace alphabet: ASCII controls, NBSP,
// U+2028/U+2029, U+3000, and repeated runs. Any separator here is legal in a
// real raw command string (bash/exec_command) or inside a JSON-encoded argv
// (shell, where Go's encoder emits U+2028/U+2029 as escapes and every other
// non-control rune as raw UTF-8 bytes).
var echoSeparators = []struct {
	name string
	sep  string
}{
	{"space", " "},
	{"tab", "\t"},
	{"newline", "\n"},
	{"vertical-tab", "\v"},
	{"form-feed", "\f"},
	{"carriage-return", "\r"},
	{"nbsp", " "},
	{"line-separator", " "},
	{"paragraph-separator", " "},
	{"ideographic-space", "　"},
	{"nbsp-run", "  "},
	{"mixed-run", " \t "},
}

// echoParityFixture is one stored tool row plus its expected classification.
type echoParityFixture struct {
	name     string
	text     string
	token    string // unique ≥3-char token so trigram FTS can MATCH the row
	wantEcho bool
}

// echoParityCorpus generates the cross product of shape × separator ×
// leading/trailing runs for direct calls, plus negative controls that must
// never classify as echoes regardless of separator.
func echoParityCorpus(t *testing.T) []echoParityFixture {
	t.Helper()
	var fixtures []echoParityFixture
	n := 0
	next := func(prefix string) string {
		n++
		// Fixed width + fixed suffix: no token is a substring of another, so
		// the trigram phrase match for one fixture's token cannot hit another
		// fixture's row.
		return fmt.Sprintf("%s%03dzq", prefix, n)
	}

	for _, s := range echoSeparators {
		// bash / exec_command shapes carry the raw command string after the
		// key= prefix; separators appear between the two canonical tokens and
		// between the command and its arguments.
		cmd := "backscroll" + s.sep + "search" + s.sep + "--text" + s.sep
		for _, wrap := range []struct {
			name, pre, post string
		}{
			{"plain", "", ""},
			{"leading-run", s.sep + s.sep, ""},
			{"trailing-run", "", s.sep + s.sep},
		} {
			tok := next("paritok")
			fixtures = append(fixtures, echoParityFixture{
				name: fmt.Sprintf("bash/%s/%s", s.name, wrap.name),
				text: wrap.pre + "Bash command=" + cmd + tok + wrap.post, token: tok, wantEcho: true,
			})
			tok = next("paritok")
			fixtures = append(fixtures, echoParityFixture{
				name: fmt.Sprintf("exec_command/%s/%s", s.name, wrap.name),
				text: wrap.pre + "exec_command cmd=" + cmd + tok + wrap.post, token: tok, wantEcho: true,
			})
			// shell shape: the separator lives inside the JSON-encoded argv,
			// serialized through a real json.Marshal + SerializeToolInput
			// round-trip so the stored text carries the encoder's escaping.
			tok = next("paritok")
			raw, err := json.Marshal(map[string]any{"command": []string{"sh", "-c", cmd + tok}})
			if err != nil {
				t.Fatal(err)
			}
			fixtures = append(fixtures, echoParityFixture{
				name:     fmt.Sprintf("shell/%s/%s", s.name, wrap.name),
				text:     wrap.pre + readers.SerializeToolInput("shell", raw) + wrap.post,
				token:    tok,
				wantEcho: true,
			})
		}
	}

	// Negative controls: near-miss shapes that no path may classify as echo.
	negatives := []struct{ name, text string }{
		{"bash/status", "Bash command=backscroll status"},
		{"bash/searcher", "Bash command=backscroll searcher"},
		{"bash/abs-path", "Bash command=/usr/local/bin/backscroll search"},
		{"bash/env-wrapper", "Bash command=env backscroll search"},
		{"bash/folded-command-key", "Bash Command=backscroll search"},
		{"bash/folded-search", "Bash command=backscroll Search"},
		{"bash/nested-lc", `Bash command=bash -lc "backscroll search"`},
		{"exec/status", "exec_command cmd=backscroll status"},
		{"prose-mention", "run backscroll search from your shell"},
		{"bash-other-tool", "Bash command=rg backscroll ."},
	}
	for _, neg := range negatives {
		tok := next("paritok")
		fixtures = append(fixtures, echoParityFixture{
			name: neg.name, text: neg.text + " " + tok, token: tok, wantEcho: false,
		})
	}
	return fixtures
}

// TestEchoExclusionThreeWayParity stores every fixture as a zero-valued
// (pre-#80, unmarked) tool row and asserts the three exclusion paths agree:
// the unfiltered page predicate, the --relax unfiltered IDF count, and the
// requeue detection in PendingSearchEchoPaths.
func TestEchoExclusionThreeWayParity(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	fixtures := echoParityCorpus(t)
	paths := make(map[string]string, len(fixtures)) // token -> source_path
	var files []IndexedFile
	for i, fx := range fixtures {
		path := fmt.Sprintf("/parity-%d.jsonl", i)
		paths[fx.token] = path
		files = append(files, IndexedFile{
			SourcePath: path, Source: "session", Hash: fx.token,
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: fx.token, Role: "assistant", ContentType: "tool",
				Text: fx.text,
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	// Zero out provenance: pre-#80 writers stored search_echo=0, the state in
	// which every path must fall back to the serialized-text chokepoint.
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo = 0 WHERE content_type = 'tool'`); err != nil {
		t.Fatal(err)
	}

	pending, err := db.PendingSearchEchoPaths()
	if err != nil {
		t.Fatal(err)
	}
	pendingSet := make(map[string]bool, len(pending))
	for _, p := range pending {
		pendingSet[p] = true
	}

	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			page := isDirectBackscrollSearchEcho(SearchResult{ContentType: "tool", Text: fx.text})
			idf, err := db.recallFrequency(recallTerm{text: fx.token}, "")
			if err != nil {
				t.Fatal(err)
			}
			idfExcluded := idf == 0
			requeued := pendingSet[paths[fx.token]]
			if page != fx.wantEcho || idfExcluded != fx.wantEcho || requeued != fx.wantEcho {
				t.Fatalf("three-way divergence for %q: page=%v idfExcluded=%v(idf=%d) requeued=%v, want echo=%v",
					fx.text, page, idfExcluded, idf, requeued, fx.wantEcho)
			}
		})
	}
}
