package storage

import (
	"fmt"
	"testing"

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
