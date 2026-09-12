package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/readers"
)

func TestRelaxationUnfilteredIDFExcludesQueryEchoes(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	files := []IndexedFile{
		{
			SourcePath: "/target.jsonl", Source: "session", Project: "alpha", Hash: "target",
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: "target", Role: "assistant", ContentType: "text",
				Text: "violet handshake quartz marker", Timestamp: "2026-01-01T00:00:00Z",
			}},
		},
	}
	for i := 0; i < 4; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/noise-%d.jsonl", i), Source: "session", Project: "alpha", Hash: "noise",
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: fmt.Sprintf("noise-%d", i), Role: "assistant", ContentType: "text",
				Text: "adaptation rollout distractor", Timestamp: "2026-01-01T00:00:00Z",
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	baseline, _, err := db.SearchRelaxed("violet handshake adaptation", models.SearchOptions{AllProjects: true, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != 1 || baseline[0].SourcePath != "/target.jsonl" || fmt.Sprint(baseline[0].DroppedTerms) != "[adaptation]" {
		t.Fatalf("control without echoes lost target: %+v", baseline)
	}

	var echoes []IndexedFile
	for i := 0; i < 8; i++ {
		echoes = append(echoes, IndexedFile{
			SourcePath: fmt.Sprintf("/echo-%d.jsonl", i), Source: "session", Project: "alpha", Hash: "echo",
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: fmt.Sprintf("echo-%d", i), Role: "assistant", ContentType: "tool",
				Text:       "Bash command=backscroll search --text 'violet handshake'",
				Timestamp:  "2026-01-01T00:00:00Z",
				SearchEcho: true,
			}},
		})
	}
	if err := db.SyncFiles(echoes); err != nil {
		t.Fatal(err)
	}

	violet, err := db.recallFrequency(recallTerm{text: "violet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	handshake, err := db.recallFrequency(recallTerm{text: "handshake"}, "")
	if err != nil {
		t.Fatal(err)
	}
	adaptation, err := db.recallFrequency(recallTerm{text: "adaptation"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if violet != 1 || handshake != 1 || adaptation != 4 {
		t.Fatalf("unfiltered IDF counted query echoes: violet=%d handshake=%d adaptation=%d", violet, handshake, adaptation)
	}

	got, stages, err := db.SearchRelaxed("violet handshake adaptation", models.SearchOptions{AllProjects: true, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range got {
		if row.SearchEcho || isDirectBackscrollSearchEcho(row) {
			t.Fatalf("echo leaked into unfiltered relaxed results: stages=%v row=%+v", stages, row)
		}
	}
	if len(got) != 1 || got[0].SourcePath != "/target.jsonl" || fmt.Sprint(got[0].DroppedTerms) != "[adaptation]" {
		t.Fatalf("query-echo IDF changed relaxed recall: stages=%v results=%+v", stages, got)
	}
}

func TestRelaxationUnfilteredIDFExcludesTextFallbackEchoes(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)
	files := []IndexedFile{{
		SourcePath: "/target.jsonl", Source: "session", Hash: "target",
		Messages: []IndexedMessage{{UUID: "target", Role: "assistant", ContentType: "text", Text: "violet handshake quartz marker"}},
	}}
	for i := 0; i < 4; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/noise-%d.jsonl", i), Source: "session", Hash: "noise",
			Messages: []IndexedMessage{{UUID: fmt.Sprintf("noise-%d", i), Role: "assistant", ContentType: "text", Text: "adaptation rollout distractor"}},
		})
	}
	for i := 0; i < 8; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/pi-echo-%d.jsonl", i), Source: "session", Hash: "echo",
			Messages: []IndexedMessage{{
				UUID: fmt.Sprintf("pi-echo-%d", i), Role: "assistant", ContentType: "tool",
				Text: "Bash command=backscroll search --text 'violet handshake'",
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	got, stages, err := db.SearchRelaxed("violet handshake adaptation", models.SearchOptions{AllProjects: true, Limit: 20})
	if err != nil || len(got) != 1 || got[0].SourcePath != "/target.jsonl" || fmt.Sprint(got[0].DroppedTerms) != "[adaptation]" {
		t.Fatalf("text-fallback echoes inverted IDF: stages=%v results=%+v err=%v", stages, got, err)
	}
}

func TestRelaxationUnfilteredIDFExcludesZeroValuedCodexShellEchoes(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	files := []IndexedFile{
		{
			SourcePath: "/target.jsonl", Source: "session", Project: "alpha", Hash: "target",
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: "target", Role: "assistant", ContentType: "text",
				Text: "violet handshake quartz marker", Timestamp: "2026-01-01T00:00:00Z",
			}},
		},
	}
	for i := 0; i < 4; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/noise-%d.jsonl", i), Source: "session", Project: "alpha", Hash: "noise",
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: fmt.Sprintf("noise-%d", i), Role: "assistant", ContentType: "text",
				Text: "adaptation rollout distractor", Timestamp: "2026-01-01T00:00:00Z",
			}},
		})
	}
	// Pre-#80 Codex writers stored search_echo=0 with the JSON-encoded shell
	// argv as text. The pure-SQL echo predicate cannot see this form; both the
	// Go predicate and recallFrequency's strict-Go subtraction must exclude it.
	for i := 0; i < 8; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/shell-echo-%d.jsonl", i), Source: "session", Project: "alpha", Hash: fmt.Sprintf("shell-echo-%d", i),
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: fmt.Sprintf("shell-echo-%d", i), Role: "assistant", ContentType: "tool",
				Text:      shellText(t, "bash", "-lc", "backscroll search --text 'violet handshake'"),
				Timestamp: "2026-01-01T00:00:00Z",
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE search_items SET search_echo=0 WHERE content_type='tool'`); err != nil {
		t.Fatal(err)
	}

	violet, err := db.recallFrequency(recallTerm{text: "violet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	handshake, err := db.recallFrequency(recallTerm{text: "handshake"}, "")
	if err != nil {
		t.Fatal(err)
	}
	adaptation, err := db.recallFrequency(recallTerm{text: "adaptation"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if violet != 1 || handshake != 1 || adaptation != 4 {
		t.Fatalf("unfiltered IDF counted zero-valued shell echoes: violet=%d handshake=%d adaptation=%d", violet, handshake, adaptation)
	}

	got, stages, err := db.SearchRelaxed("violet handshake adaptation", models.SearchOptions{AllProjects: true, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range got {
		if isDirectBackscrollSearchEcho(row) {
			t.Fatalf("shell echo leaked into unfiltered relaxed results: stages=%v row=%+v", stages, row)
		}
	}
	if len(got) != 1 || got[0].SourcePath != "/target.jsonl" || fmt.Sprint(got[0].DroppedTerms) != "[adaptation]" {
		t.Fatalf("zero-valued shell echoes inverted --relax IDF: stages=%v results=%+v", stages, got)
	}
}

func TestRelaxationUnfilteredIDFStillCountsLegitimateTools(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)
	files := []IndexedFile{{
		SourcePath: "/target.jsonl", Source: "session", Hash: "target",
		Messages: []IndexedMessage{{UUID: "target", Role: "assistant", ContentType: "text", Text: "violet handshake quartz marker"}},
	}}
	for i := 0; i < 4; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/noise-%d.jsonl", i), Source: "session", Hash: "noise",
			Messages: []IndexedMessage{{UUID: fmt.Sprintf("noise-%d", i), Role: "assistant", ContentType: "text", Text: "adaptation rollout distractor"}},
		})
	}
	for i := 0; i < 8; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/rg-%d.jsonl", i), Source: "session", Hash: "rg",
			Messages: []IndexedMessage{{
				UUID: fmt.Sprintf("rg-%d", i), Role: "assistant", ContentType: "tool",
				Text: "Bash command=rg violet handshake /tmp",
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	violet, err := db.recallFrequency(recallTerm{text: "violet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if violet != 9 {
		t.Fatalf("legitimate tool rows must still count toward unfiltered IDF: violet=%d", violet)
	}
}

// An echo-only term never appears in a returnable unfiltered row, so unfiltered
// IDF is 0. --relax therefore drops it last (highest IDF), and at the two-term
// floor the extra term can prevent recovery. Pre-#81 counted those echo rows
// and dropped the term first by accident; that is not the documented rule.
func TestRelaxationUnfilteredIDFTreatsEchoOnlyTermsAsAbsent(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	files := []IndexedFile{{
		SourcePath: "/target.jsonl", Source: "session", Project: "alpha", Hash: "target",
		Messages: []IndexedMessage{{
			Ordinal: 0, UUID: "target", Role: "assistant", ContentType: "text",
			Text: "violet handshake quartz marker", Timestamp: "2026-01-01T00:00:00Z",
		}},
	}}
	for i := 0; i < 8; i++ {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/echo-%d.jsonl", i), Source: "session", Project: "alpha", Hash: "echo",
			Messages: []IndexedMessage{{
				Ordinal: 0, UUID: fmt.Sprintf("echo-%d", i), Role: "assistant", ContentType: "tool",
				Text:       "Bash command=backscroll search --text 'zebra rollout'",
				Timestamp:  "2026-01-01T00:00:00Z",
				SearchEcho: true,
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}

	violet, err := db.recallFrequency(recallTerm{text: "violet"}, "")
	if err != nil {
		t.Fatal(err)
	}
	handshake, err := db.recallFrequency(recallTerm{text: "handshake"}, "")
	if err != nil {
		t.Fatal(err)
	}
	zebra, err := db.recallFrequency(recallTerm{text: "zebra"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if violet != 1 || handshake != 1 || zebra != 0 {
		t.Fatalf("echo-only term must have unfiltered DF=0: violet=%d handshake=%d zebra=%d", violet, handshake, zebra)
	}

	got, stages, err := db.SearchRelaxed("violet handshake zebra", models.SearchOptions{AllProjects: true, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("echo-only DF=0 term must not be dropped first to recover the target: stages=%v results=%+v", stages, got)
	}
	if fmt.Sprint(stages) != "[strict drop-terms/1]" {
		t.Fatalf("expected one drop of a real term before the two-term floor: stages=%v", stages)
	}
}

// TestEchoBoundaryCasesThreeWayParity pins the accepted/rejected boundary for
// every hand-picked lookalike and asserts the three exclusion paths agree on
// each: the page predicate, the unfiltered IDF count, and the requeue
// detection. There is deliberately no SQL-side shape predicate left to
// compare against — SQL applies only the exact search_echo check and the
// broad substring prefilter, and the strict decision is shared Go code.
func TestEchoBoundaryCasesThreeWayParity(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	type row struct {
		name        string
		contentType string
		text        string
		echo        bool
		nullEcho    bool
		unique      string
		wantEcho    bool
	}
	// Shell fixtures go through a real json.Marshal + SerializeToolInput
	// round-trip so the stored text carries the encoder's actual escaping.
	shellFull := func(argv []string) string {
		raw, err := json.Marshal(map[string]any{
			"additional_permissions": "read",
			"command":                argv,
			"timeout_ms":             1000,
		})
		if err != nil {
			t.Fatal(err)
		}
		return readers.SerializeToolInput("shell", raw)
	}
	cases := []row{
		{name: "canonical Bash", contentType: "tool", text: "Bash command=backscroll search --text boundtok00", unique: "boundtok00", wantEcho: true},
		{name: "lowercase bash", contentType: "tool", text: "bash command=backscroll search --text boundtok01", unique: "boundtok01", wantEcho: true},
		{name: "path prefix", contentType: "tool", text: "/private/tmp/bin/backscroll search --text boundtok02", unique: "boundtok02"},
		{name: "command path", contentType: "tool", text: "Bash command=/private/tmp/bin/backscroll search --text boundtok03", unique: "boundtok03"},
		{name: "env wrapper", contentType: "tool", text: "Bash command=env backscroll search --text boundtok04", unique: "boundtok04"},
		{name: "bash -lc", contentType: "tool", text: `Bash command=bash -lc "backscroll search --text boundtok05"`, unique: "boundtok05"},
		{name: "rg", contentType: "tool", text: "Bash command=rg boundtok06 .", unique: "boundtok06"},
		{name: "status", contentType: "tool", text: "Bash command=backscroll status boundtok07", unique: "boundtok07"},
		{name: "searcher", contentType: "tool", text: "Bash command=backscroll searcher boundtok08", unique: "boundtok08"},
		{name: "error prefix", contentType: "tool", text: "error: Bash command=backscroll search failed boundtok09", unique: "boundtok09"},
		{name: "prose lookalike", contentType: "text", text: "Bash command=backscroll search --text boundtok10", unique: "boundtok10"},
		{name: "null echo legitimate", contentType: "tool", text: "Bash command=rg boundtok11 /tmp", nullEcho: true, unique: "boundtok11"},
		{name: "null echo fallback", contentType: "tool", text: "Bash command=backscroll search --text boundtok12", nullEcho: true, unique: "boundtok12", wantEcho: true},
		{name: "proven echo without prefix", contentType: "tool", text: "Bash command=rg boundtok13 /tmp", echo: true, unique: "boundtok13", wantEcho: true},
		{name: "uppercase BASH", contentType: "tool", text: "BASH command=backscroll search --text boundtok14", unique: "boundtok14", wantEcho: true},
		{name: "folded command=", contentType: "tool", text: "Bash Command=backscroll search --text boundtok15", unique: "boundtok15"},
		{name: "folded Search", contentType: "tool", text: "Bash command=backscroll Search --text boundtok16", unique: "boundtok16"},
		{name: "canonical exec_command", contentType: "tool", text: `exec_command cmd=backscroll search --text boundtok17 command=[["unused"]]`, unique: "boundtok17", wantEcho: true},
		// Codex shell wrapper form: excluded by the same shared Go predicate,
		// and by recallFrequency's broad-prefilter + strict-Go subtraction.
		{name: "canonical shell", contentType: "tool", text: shellText(t, "bash", "-lc", "backscroll search --text boundtok20"), unique: "boundtok20", wantEcho: true},
		{name: "shell extra sorted keys", contentType: "tool", text: shellFull([]string{"sh", "-c", "backscroll search --text boundtok21"}), unique: "boundtok21", wantEcho: true},
		{name: "shell /bin/bash path", contentType: "tool", text: shellText(t, "/bin/bash", "-lc", "backscroll search --text boundtok22"), unique: "boundtok22", wantEcho: true},
		// Real NBSP (U+00A0, bytes 0xC2 0xA0) between the canonical tokens:
		// Go's json encoder emits NBSP as raw UTF-8 bytes (it escapes only
		// control chars and U+2028/2029), and strings.Fields splits on it.
		{name: "shell NBSP separator", contentType: "tool", text: shellText(t, "sh", "-c", "backscroll\u00a0search --text boundtok23"), unique: "boundtok23", wantEcho: true},
		// JSON control escapes (\t here) stay two-byte sequences in the
		// serialized text, so with no other argv whitespace the whole row has
		// only two strings.Fields tokens — it must not fall below a token-count
		// floor before the shell check, or pages would keep a row the IDF path
		// (which has no such floor) excludes.
		{name: "shell JSON-escaped tab separators", contentType: "tool", text: shellText(t, "sh", "-c", "backscroll\tsearch\t--text\tboundtok31"), unique: "boundtok31", wantEcho: true},
		{name: "null echo shell fallback", contentType: "tool", text: shellText(t, "bash", "-lc", "backscroll search --text boundtok24"), nullEcho: true, unique: "boundtok24", wantEcho: true},
		{name: "shell wrong command", contentType: "tool", text: shellText(t, "sh", "-c", "rg boundtok25 /tmp"), unique: "boundtok25"},
		{name: "shell wrong flag", contentType: "tool", text: shellText(t, "sh", "-x", "backscroll search --text boundtok26"), unique: "boundtok26"},
		{name: "shell extra argv element", contentType: "tool", text: shellTextN(t, "sh", "-c", "backscroll search --text boundtok27", "bar"), unique: "boundtok27"},
		{name: "shell non-search call", contentType: "tool", text: shellText(t, "sh", "-c", "backscroll status boundtok28"), unique: "boundtok28"},
		{name: "shell env wrapper inside argv", contentType: "tool", text: shellText(t, "sh", "-c", "env backscroll search --text boundtok29"), unique: "boundtok29"},
		{name: "shell absolute path inside argv", contentType: "tool", text: shellText(t, "sh", "-c", "/usr/local/bin/backscroll search --text boundtok30"), unique: "boundtok30"},
	}

	var files []IndexedFile
	for i, tc := range cases {
		files = append(files, IndexedFile{
			SourcePath: fmt.Sprintf("/bound-%d.jsonl", i), Source: "session", Hash: tc.unique,
			Messages: []IndexedMessage{{
				UUID: tc.unique, Role: "assistant", ContentType: tc.contentType,
				Text: tc.text, SearchEcho: tc.echo,
			}},
		})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	for i, tc := range cases {
		if !tc.nullEcho {
			continue
		}
		path := fmt.Sprintf("/bound-%d.jsonl", i)
		if _, err := db.db.Exec("UPDATE search_items SET search_echo = NULL WHERE source_path = ?", path); err != nil {
			t.Fatalf("null search_echo for %s: %v", tc.name, err)
		}
	}

	pending, err := db.PendingSearchEchoPaths()
	if err != nil {
		t.Fatal(err)
	}
	pendingSet := make(map[string]bool, len(pending))
	for _, p := range pending {
		pendingSet[p] = true
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var result SearchResult
			var echo int
			err := db.db.QueryRow(
				"SELECT si.content_type, COALESCE(si.search_echo, 0), si.text FROM search_items si WHERE si.uuid = ?",
				tc.unique,
			).Scan(&result.ContentType, &echo, &result.Text)
			if err != nil {
				t.Fatal(err)
			}
			result.SearchEcho = echo != 0
			gotGo := isDirectBackscrollSearchEcho(result)
			if gotGo != tc.wantEcho {
				t.Fatalf("Go predicate = %v, want %v (echo=%d text=%q)", gotGo, tc.wantEcho, echo, result.Text)
			}

			got, err := db.recallFrequency(recallTerm{text: tc.unique}, "")
			if err != nil {
				t.Fatal(err)
			}
			wantDF := 1
			if tc.wantEcho {
				wantDF = 0
			}
			if got != wantDF {
				t.Fatalf("unfiltered IDF(%s)=%d, want %d", tc.unique, got, wantDF)
			}

			// Requeue detection: NULL-backlog rows are always requeued; a
			// zero-valued row is requeued iff the chokepoint accepts its text;
			// an already-proven echo row is classified and never requeued.
			path := fmt.Sprintf("/bound-%d.jsonl", i)
			wantRequeue := tc.nullEcho || (tc.wantEcho && !tc.echo)
			if pendingSet[path] != wantRequeue {
				t.Fatalf("requeue(%s) = %v, want %v (echo=%d nullEcho=%v text=%q)", tc.name, pendingSet[path], wantRequeue, echo, tc.nullEcho, result.Text)
			}
		})
	}
}

func TestRecallFrequencyUnfilteredSQLMatchesGoScan(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	const n = 8000
	filler := strings.Repeat("lorem output line about build errors and paths /usr/local/share ", 24)
	msgs := make([]IndexedMessage, n)
	for i := 0; i < n; i++ {
		text := "Bash command=rg commonterm /tmp\n" + filler
		echo := false
		if i%20 == 0 {
			// Zero-valued Codex shell echo (pre-#80 state): excluded by the Go
			// predicate and by recallFrequency's strict-Go subtraction, but not
			// by the pure-SQL predicate — proving both paths agree at scale.
			text = shellText(t, "bash", "-lc", "backscroll search --text commonterm") + "\n" + filler
		} else if i%10 == 0 {
			text = "Bash command=backscroll search --text commonterm\n" + filler
			echo = true
		}
		msgs[i] = IndexedMessage{
			Ordinal: i, UUID: fmt.Sprintf("mix-%d", i), Role: "assistant",
			ContentType: "tool", Text: text, SearchEcho: echo,
		}
	}
	if err := db.SyncFiles([]IndexedFile{{
		SourcePath: "/mix.jsonl", Source: "session", Hash: "mix", Messages: msgs,
	}}); err != nil {
		t.Fatal(err)
	}

	term := recallTerm{text: "commonterm"}
	sqlCount, err := db.recallFrequency(term, "")
	if err != nil {
		t.Fatal(err)
	}
	goCount, err := recallFrequencyGoScan(db, term)
	if err != nil {
		t.Fatal(err)
	}
	if sqlCount != goCount {
		t.Fatalf("SQL IDF=%d Go-scan IDF=%d", sqlCount, goCount)
	}
	want := n - n/10
	if sqlCount != want {
		t.Fatalf("unfiltered IDF=%d, want %d non-echo rows", sqlCount, want)
	}

	const rounds = 5
	sqlStart := time.Now()
	for i := 0; i < rounds; i++ {
		if _, err := db.recallFrequency(term, ""); err != nil {
			t.Fatal(err)
		}
	}
	sqlElapsed := time.Since(sqlStart) / rounds
	goStart := time.Now()
	for i := 0; i < rounds; i++ {
		if _, err := recallFrequencyGoScan(db, term); err != nil {
			t.Fatal(err)
		}
	}
	goElapsed := time.Since(goStart) / rounds
	t.Logf("unfiltered IDF n=%d sql=%s go-scan=%s", n, sqlElapsed, goElapsed)
}

func recallFrequencyGoScan(db *Database, term recallTerm) (int, error) {
	exprMessages := recallExpression([]recallTerm{term}, "messages_fts")
	exprTool := recallExpression([]recallTerm{term}, "tool_fts")
	rows, err := db.db.Query(
		`SELECT si.content_type, COALESCE(si.search_echo, 0), si.text FROM (
			SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?
			UNION
			SELECT rowid FROM tool_fts WHERE tool_fts MATCH ?
		) matched JOIN search_items si ON si.id = matched.rowid`,
		exprMessages, exprTool,
	)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	var count int
	for rows.Next() {
		var result SearchResult
		var echo int
		if err := rows.Scan(&result.ContentType, &echo, &result.Text); err != nil {
			return 0, err
		}
		result.SearchEcho = echo != 0
		if isDirectBackscrollSearchEcho(result) {
			continue
		}
		count++
	}
	return count, rows.Err()
}
