package storage

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
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

func TestRecallFrequencySQLEchoPredicatePreservesBoundaries(t *testing.T) {
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

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var result SearchResult
			var echo int
			var sqlEcho int
			err := db.db.QueryRow(
				"SELECT si.content_type, COALESCE(si.search_echo, 0), si.text, CASE WHEN "+directBackscrollSearchEchoSQL("si")+" THEN 1 ELSE 0 END FROM search_items si WHERE si.uuid = ?",
				tc.unique,
			).Scan(&result.ContentType, &echo, &result.Text, &sqlEcho)
			if err != nil {
				t.Fatal(err)
			}
			result.SearchEcho = echo != 0
			gotGo := isDirectBackscrollSearchEcho(result)
			if gotGo != tc.wantEcho {
				t.Fatalf("Go predicate = %v, want %v (echo=%d text=%q)", gotGo, tc.wantEcho, echo, result.Text)
			}
			if (sqlEcho == 1) != tc.wantEcho {
				t.Fatalf("SQL predicate = %d, want echo=%v (echo=%d text=%q)", sqlEcho, tc.wantEcho, echo, result.Text)
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
		if i%10 == 0 {
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
