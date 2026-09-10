package storage

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

func relaxationDB(t *testing.T) *Database {
	t.Helper()
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)
	files := []IndexedFile{
		{SourcePath: "/target.jsonl", Source: "session", Project: "alpha", Hash: "target", Tags: []string{"testing"}, Messages: []IndexedMessage{{Ordinal: 0, UUID: "target", Role: "assistant", ContentType: "text", Text: "violet handshake quartz marker", Timestamp: "2026-01-01T00:00:00Z"}}},
		{SourcePath: "/reverse.jsonl", Source: "session", Project: "beta", Hash: "reverse", Messages: []IndexedMessage{{Ordinal: 0, UUID: "reverse", Role: "user", ContentType: "text", Text: "handshake violet quartz marker", Timestamp: "2026-01-01T00:00:00Z"}}},
		{SourcePath: "/tool.jsonl", Source: "session", Project: "alpha", Hash: "tool", Messages: []IndexedMessage{{Ordinal: 0, UUID: "tool", Role: "tool", ContentType: "tool", Text: "violet handshake quartz marker", Timestamp: "2026-01-01T00:00:00Z"}}},
	}
	for i := 0; i < 4; i++ {
		files = append(files, IndexedFile{SourcePath: fmt.Sprintf("/noise-%d.jsonl", i), Source: "session", Project: "alpha", Hash: "noise", Messages: []IndexedMessage{{Ordinal: 0, UUID: fmt.Sprintf("noise-%d", i), Role: "assistant", ContentType: "text", Text: "adaptation rollout distractor", Timestamp: "2026-01-01T00:00:00Z"}}})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRelaxationStagesAndProtectedCore(t *testing.T) {
	db := relaxationDB(t)
	t.Run("punctuation cannot satisfy the two-term floor", func(t *testing.T) {
		results, stages, err := db.SearchRelaxed("handshake adaptation ...", models.SearchOptions{SourcePath: "/target.jsonl", ContentType: "text", Limit: 5})
		if err == nil || !strings.Contains(err.Error(), "has no searchable characters") || len(results) != 0 || len(stages) != 0 {
			t.Fatalf("punctuation must be rejected before any stage, not allow single-term recall: err=%v stages=%v results=%+v", err, stages, results)
		}
	})
	for _, tc := range []struct {
		query string
		want  bool
		drops []string
		tries []string
	}{
		{"violet handshake adaptation", true, []string{"adaptation"}, []string{"strict", "drop-terms/1"}},
		{"rollout adaptation violet handshake", true, []string{"rollout", "adaptation"}, []string{"strict", "drop-terms/1", "drop-terms/2"}},
		{"+adaptation rollout violet handshake", false, nil, []string{"strict", "drop-terms/1"}},
		{"violet nonexistent", false, nil, []string{"strict"}},
		{"violet violet nonexistent", false, nil, []string{"strict"}},
		{"violet Violet nonexistent", false, nil, []string{"strict"}},
		{"+violet +handshake", true, nil, []string{"strict"}},
		{`"violet handshake" quartz marker adaptation`, true, []string{"adaptation"}, []string{"strict", "drop-terms/1"}},
		{`"handshake violet" quartz marker adaptation`, false, nil, []string{"strict", "drop-terms/1"}},
		{"+missing violet handshake adaptation", false, nil, []string{"strict", "drop-terms/1"}},
		{"remembered connector survive restarts", false, nil, []string{"strict", "drop-terms/1", "drop-terms/2"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			results, stages, err := db.SearchRelaxed(tc.query, models.SearchOptions{SourcePath: "/target.jsonl", ContentType: "text", Limit: 5})
			if err != nil {
				t.Fatal(err)
			}
			if (len(results) == 1) != tc.want || len(results) > 1 {
				t.Fatalf("want target=%t, got %#v", tc.want, results)
			}
			if !reflect.DeepEqual(stages, tc.tries) {
				t.Fatalf("stages=%v want=%v", stages, tc.tries)
			}
			if tc.want {
				if !reflect.DeepEqual(results[0].DroppedTerms, tc.drops) {
					t.Fatalf("drops=%v want=%v", results[0].DroppedTerms, tc.drops)
				}
				if (results[0].MatchStage == "drop-terms") != (len(tc.drops) > 0) {
					t.Fatalf("bad provenance: %#v", results[0])
				}
			}
		})
	}
}

func TestRelaxationPreservesEveryFilter(t *testing.T) {
	db := relaxationDB(t)
	after := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	before := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	for _, opts := range []models.SearchOptions{
		{Project: "missing"}, {SourcePath: "/missing.jsonl"}, {Source: "memory"},
		{Role: "system"}, {ContentType: "reasoning"}, {Tag: "absent"},
		{After: &after}, {Before: &before},
		{Project: "beta", Role: "assistant"}, {SourcePath: "/target.jsonl", ContentType: "tool"},
	} {
		results, _, err := db.SearchRelaxed("violet handshake adaptation", opts)
		if err != nil || len(results) != 0 {
			t.Fatalf("filter escaped: opts=%+v err=%v results=%+v", opts, err, results)
		}
	}
	results, _, err := db.SearchRelaxed("violet handshake adaptation", models.SearchOptions{Project: "alpha", ContentType: "text", Role: "assistant", Tag: "testing", Source: "session", SourcePath: "/target*"})
	if err != nil || len(results) != 1 || results[0].SourcePath != "/target.jsonl" {
		t.Fatalf("matching filters lost target: %v %+v", err, results)
	}
}

func TestRelaxationToolFilterAndPagination(t *testing.T) {
	db := relaxationDB(t)
	// Tool IDF is measured only on the trigram stream. Add frequent tool noise
	// so adaptation, not the rare two-term core, is the term that can be dropped.
	var files []IndexedFile
	for i := 0; i < 3; i++ {
		files = append(files, IndexedFile{SourcePath: fmt.Sprintf("/tool-noise-%d.jsonl", i), Source: "session", Hash: "noise", Messages: []IndexedMessage{{Ordinal: 0, UUID: fmt.Sprintf("tool-noise-%d", i), Role: "tool", ContentType: "tool", Text: "adaptation rollout distractor"}}})
	}
	if err := db.SyncFiles(files); err != nil {
		t.Fatal(err)
	}
	results, _, err := db.SearchRelaxed("violet handshake adaptation", models.SearchOptions{ContentType: "tool"})
	if err != nil || len(results) != 1 || results[0].SourcePath != "/tool.jsonl" {
		t.Fatalf("tool-only relaxation: %v %+v", err, results)
	}
	for _, query := range []string{"violet handshake", "violet handshake adaptation"} {
		results, stages, err := db.SearchRelaxed(query, models.SearchOptions{SourcePath: "/target.jsonl", Offset: 1, Limit: 1})
		if err != nil || len(results) != 0 {
			t.Fatalf("exhausted page should stay empty: %v %+v", err, results)
		}
		wantStages := 1
		if strings.HasSuffix(query, "adaptation") {
			wantStages = 2
		}
		if len(stages) != wantStages {
			t.Fatalf("pagination triggered extra relaxation: %v", stages)
		}
	}
}

func TestRelaxationValidationAndLiterals(t *testing.T) {
	for _, query := range []string{"", "  ", "+", "+ term", `""`, `"unclosed`, `"phrase"suffix`, strings.Repeat("word ", 33) + "+", "...", "&&", "->", "--", "::", "||", "+...", `"... &&"`, "✨"} {
		if err := ValidateRelaxationQuery(query); err == nil {
			t.Errorf("expected invalid query %q", query)
		}
	}
	for _, query := range []string{"a", "7", "é", "東京", "١", "+東京", "--dry-run", "/tmp/a-b", "foo::bar", `"東京 7"`} {
		if err := ValidateRelaxationQuery(query); err != nil {
			t.Errorf("letter/digit-bearing unit rejected %q: %v", query, err)
		}
	}
	var long []string
	for i := 0; i < 33; i++ {
		long = append(long, fmt.Sprintf("term%d", i))
	}
	if err := ValidateRelaxationQuery(strings.Join(long, " ")); err == nil {
		t.Fatal("unbounded query accepted")
	}
	terms, err := parseRecallTerms(`+violet Violet "handshake quartz" "a ""quote""" OR /tmp/a-b --flag`)
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 6 || !terms[0].protected || !terms[1].phrase {
		t.Fatalf("deduplication/protection lost: %+v", terms)
	}
	want := `"violet"* "handshake quartz" "a ""quote""" "OR"* "/tmp/a-b"* "--flag"*`
	if got := recallExpression(terms, "messages_fts"); got != want {
		t.Fatalf("literal query=%s want=%s", got, want)
	}
	if got := recallExpression(terms[:1], "tool_fts"); got != `"violet"` {
		t.Fatalf("trigram query=%s", got)
	}
	// A protected duplicate later in a query must protect the earlier unit.
	terms, err = parseRecallTerms("violet +VIOLET")
	if err != nil || len(terms) != 1 || !terms[0].protected {
		t.Fatalf("later keep marker lost: %+v %v", terms, err)
	}
}

func TestRelaxationErrorsAreNotZeroResultFallbacks(t *testing.T) {
	db := relaxationDB(t)
	if _, _, err := db.SearchRelaxed(`"unclosed`, models.SearchOptions{}); err == nil {
		t.Fatal("invalid direct query accepted")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.SearchRelaxed("violet handshake adaptation", models.SearchOptions{}); err == nil {
		t.Fatal("database failure treated as a retrieval miss")
	}
	if _, err := db.recallFrequency(recallTerm{text: "violet"}, "text"); err == nil {
		t.Fatal("frequency failure ignored")
	}
}
