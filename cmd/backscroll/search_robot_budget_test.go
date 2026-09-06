package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestSearchRobotLinesMinimalUsesBoundedSnippet(t *testing.T) {
	results := []storage.SearchResult{{
		Source:     "session",
		SourcePath: "/tmp/session.jsonl",
		Role:       "assistant",
		Text:       "full private content that must not appear",
		Snippet:    "bounded snippet",
		Score:      1.25,
		Timestamp:  time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC),
		Project:    "backscroll",
	}}

	got := searchRobotLines(results, "minimal", 0)
	want := []string{
		"result_0_filepath=/tmp/session.jsonl",
		"result_0_content=bounded snippet",
		"result_0_score=1.25",
		"result_0_role=assistant",
		"result_0_timestamp=2026-09-06T08:00:00Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("minimal robot projection mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
	if strings.Contains(strings.Join(got, "\n"), "full private content") {
		t.Fatal("minimal robot projection leaked full content")
	}
}

func TestSearchRobotLinesFullUsesCompleteContent(t *testing.T) {
	results := []storage.SearchResult{{
		Source:      "session",
		SourcePath:  "/tmp/session.jsonl",
		Role:        "assistant",
		Text:        "full content",
		Snippet:     "short snippet",
		Score:       0.5,
		Timestamp:   time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC),
		Project:     "backscroll",
		ContentType: "text",
	}}

	got := searchRobotLines(results, "full", 0)
	want := []string{
		"result_0_source=session",
		"result_0_role=assistant",
		"result_0_filepath=/tmp/session.jsonl",
		"result_0_content=full content",
		"result_0_project=backscroll",
		"result_0_content_type=text",
		"result_0_timestamp=2026-09-06T08:00:00Z",
		"result_0_score=0.50",
		"result_0_rank=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("full robot projection mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestSearchRobotLinesBudgetsWholeResultsAndReportsOmission(t *testing.T) {
	results := []storage.SearchResult{
		{SourcePath: "/a", Role: "assistant", Snippet: "alpha", Score: 1},
		{SourcePath: "/b", Role: "assistant", Snippet: "beta", Score: 2},
		{SourcePath: "/c", Role: "assistant", Snippet: "gamma", Score: 3},
	}

	got := searchRobotLines(results, "minimal", 7)
	want := []string{
		"result_0_filepath=/a",
		"result_0_content=alpha",
		"result_0_score=1.00",
		"result_0_role=assistant",
		"result_0_timestamp=",
		"result_1_truncated=true",
		"result_1_omitted=2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("budgeted robot output mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestSearchRobotLinesReportsWhenFirstResultExceedsBudget(t *testing.T) {
	results := []storage.SearchResult{{
		SourcePath: "/oversized",
		Role:       "assistant",
		Snippet:    strings.Repeat("large ", 20),
		Score:      1,
	}}

	got := searchRobotLines(results, "minimal", 2)
	want := []string{
		"result_0_truncated=true",
		"result_0_omitted=1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("oversized first result mismatch\ngot:  %#v\nwant: %#v", got, want)
	}
}
