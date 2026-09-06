package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/storage"
	picokitoutput "github.com/pablontiv/picokit/output"
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

	got := searchRobotLines(results, "minimal", 9)
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
	if tokens := picokitoutput.TokenCount(strings.Join(got, "\n")); tokens > 9 {
		t.Fatalf("complete payload uses %d tokens, budget is 9", tokens)
	}
}

func TestSearchRobotLinesRoundsOnlyTheCompletePayload(t *testing.T) {
	results := []storage.SearchResult{
		{SourcePath: "/a", Role: "assistant", Snippet: "alpha", Text: "alpha"},
		{SourcePath: "/b", Role: "assistant", Snippet: "beta", Text: "beta"},
		{SourcePath: "/c", Role: "assistant", Snippet: "gamma", Text: "gamma"},
	}
	for _, tt := range []struct {
		fields  string
		budget  int
		results int
		omitted int
	}{
		{"minimal", 0, 3, 0},
		{"minimal", 1, 0, 0},
		{"minimal", 2, 0, 3},
		{"minimal", 7, 0, 3},
		{"minimal", 8, 0, 3},
		{"minimal", 9, 1, 2},
		{"minimal", 13, 1, 2},
		{"minimal", 15, 2, 1},
		{"minimal", 18, 2, 1},
		{"minimal", 19, 3, 0},
		{"full", 10, 0, 3},
		{"full", 11, 1, 2},
		{"full", 18, 1, 2},
		{"full", 20, 2, 1},
		{"full", 27, 3, 0},
	} {
		t.Run(fmt.Sprintf("%s/%d", tt.fields, tt.budget), func(t *testing.T) {
			lines := searchRobotLines(results, tt.fields, tt.budget)
			payload := strings.Join(lines, "\n")
			if tokens := picokitoutput.TokenCount(payload); tt.budget > 0 && tokens > tt.budget {
				t.Fatalf("complete payload uses %d tokens, budget is %d:\n%s", tokens, tt.budget, payload)
			}
			if got := strings.Count(payload, "_content="); got != tt.results {
				t.Fatalf("got %d complete results, want %d:\n%s", got, tt.results, payload)
			}
			fieldsPerResult := 5
			if tt.fields == "full" {
				fieldsPerResult = 7
			}
			wantLines := tt.results * fieldsPerResult
			if tt.omitted > 0 {
				wantLines += 2
				marker := fmt.Sprintf("result_%d_truncated=true\nresult_%d_omitted=%d", tt.results, tt.results, tt.omitted)
				if !strings.HasSuffix(payload, marker) {
					t.Fatalf("missing omission marker %q:\n%s", marker, payload)
				}
			}
			if len(lines) != wantLines {
				t.Fatalf("got %d lines, want %d (no partial results)", len(lines), wantLines)
			}
		})
	}
}

func TestSearchRobotBudgetWithLongIndexedMessages(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	sessions := t.TempDir()
	fullText := "budgetsentinel " + strings.Repeat("synthetic payload ", 100) + "fullonlysentinel"
	var records strings.Builder
	for i := 1; i <= 3; i++ {
		record, err := json.Marshal(map[string]any{
			"uuid": fmt.Sprintf("budget-%d", i), "type": "assistant",
			"timestamp": fmt.Sprintf("2026-09-0%dT00:00:00Z", i),
			"message":   map[string]string{"role": "assistant", "content": fullText},
		})
		if err != nil {
			t.Fatal(err)
		}
		records.Write(record)
		records.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(sessions, "budget.jsonl"), []byte(records.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := filepath.Join(os.Getenv("BACKSCROLL_CONFIG_DIR"), "backscroll", "inputs")
	if err := os.MkdirAll(inputs, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("version = 1\n[[inputs]]\nid = \"budget\"\nsource = \"session\"\nactive = true\n[inputs.discover]\nroots = [%q]\ninclude = [\"**/*.jsonl\"]\n[inputs.decode]\nformat = \"claude\"\n", sessions)
	if err := os.WriteFile(filepath.Join(inputs, "budget.inputs.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, fields := range []string{"minimal", "full"} {
		args := []string{"search", "--text", "budgetsentinel", "--all-projects", "--robot", "--fields", fields}
		unlimited, stderr, err := runCmd(args...)
		if err != nil || strings.Count(unlimited, "_content=") != 3 {
			t.Fatalf("fixture must yield three %s results: %v\nstdout: %s\nstderr: %s", fields, err, unlimited, stderr)
		}
		if got := strings.Count(unlimited, "fullonlysentinel"); (fields == "full" && got != 3) || (fields == "minimal" && got != 0) {
			t.Fatalf("wrong %s projection: full-only tail count %d", fields, got)
		}
		for _, budget := range []int{2, 7, 50, 100} {
			out, stderr, err := runCmd(append(args, "--max-tokens", fmt.Sprint(budget))...)
			if err != nil {
				t.Fatalf("budgeted %s query: %v\n%s", fields, err, stderr)
			}
			if tokens := picokitoutput.TokenCount(out); tokens > budget {
				t.Fatalf("%s complete payload uses %d tokens, budget is %d:\n%s", fields, tokens, budget, out)
			}
			if !strings.Contains(out, "_truncated=true\n") || !strings.Contains(out, "_omitted=") {
				t.Fatalf("budget %d must report omitted %s records:\n%s", budget, fields, out)
			}
		}
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
