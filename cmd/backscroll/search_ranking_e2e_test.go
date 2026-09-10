package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestSearchRanksStrongestBM25MatchFirst is the regression test for issue #77.
// FTS5 bm25() is lower-is-better (negative); the SQL previously sorted DESC,
// so the weakest match ranked first on every path and, once matches exceeded
// the page or candidate cap, the best matches were dropped entirely.
//
// The fixture needs enough non-matching documents that IDF stays positive:
// when the term appears in most documents FTS5 clamps IDF to ~0, every match
// ties near -0.000, and the inversion hides behind the tie.
func TestSearchRanksStrongestBM25MatchFirst(t *testing.T) {
	e := newQueryEchoE2E(t)
	filler := strings.Repeat("filler alpha beta gamma delta ", 60)

	e.writeRecord("strong.jsonl", "strong", "strong", 0, "widget widget widget widget", false)
	e.writeRecord("medium.jsonl", "medium", "medium", 1, "the widget was assembled by the team after lunch with some coffee and a long discussion about the schedule", false)
	e.writeRecord("weak.jsonl", "weak", "weak", 2, "widget "+filler, false)
	e.writeRecord("strong-tool.jsonl", "strong-tool", "strong-tool", 3, queryEchoToolBlock("strong-tool", "sprocket sprocket sprocket sprocket"), false)
	e.writeRecord("weak-tool.jsonl", "weak-tool", "weak-tool", 4, queryEchoToolBlock("weak-tool", "sprocket --flag one --flag two --flag three --flag four --flag five --flag six --flag seven --flag eight --flag nine --flag ten --flag eleven --flag twelve"), false)
	for i := 0; i < 20; i++ {
		e.writeRecord("nonmatch.jsonl", fmt.Sprintf("nonmatch-%02d", i), "nonmatch", 5+i, fmt.Sprintf("unrelated note %d about the weather and the deploy schedule with nothing else", i), i > 0)
		e.writeRecord("nonmatch-tool.jsonl", fmt.Sprintf("nonmatch-tool-%02d", i), "nonmatch-tool", 5+i, queryEchoToolBlock(fmt.Sprintf("nonmatch-tool-%02d", i), fmt.Sprintf("echo unrelated tool call %d", i)), i > 0)
	}
	e.run("status", "--json")

	wantProse := []string{"strong.jsonl", "medium.jsonl", "weak.jsonl"}
	wantTool := []string{"strong-tool.jsonl", "weak-tool.jsonl"}

	textOnly := e.searchJSON("widget", "text", 20)
	if got := rankedFiles(textOnly); !reflect.DeepEqual(got, wantProse) {
		t.Errorf("--content-type text order = %v, want %v", got, wantProse)
	}
	assertAscendingNonPositiveScores(t, "--content-type text", e.searchScores("widget", "text", 20))

	toolOnly := e.searchJSON("sprocket", "tool", 20)
	if got := rankedFiles(toolOnly); !reflect.DeepEqual(got, wantTool) {
		t.Errorf("--content-type tool order = %v, want %v", got, wantTool)
	}
	assertAscendingNonPositiveScores(t, "--content-type tool", e.searchScores("sprocket", "tool", 20))

	// The unfiltered path fuses both tables by rank position (RRF), so it must
	// inherit the corrected order without any change of its own.
	if got := rankedFiles(e.searchJSON("widget", "", 20)); !reflect.DeepEqual(got, wantProse) {
		t.Errorf("unfiltered RRF prose order = %v, want %v", got, wantProse)
	}
	if got := rankedFiles(e.searchJSON("sprocket", "", 20)); !reflect.DeepEqual(got, wantTool) {
		t.Errorf("unfiltered RRF tool order = %v, want %v", got, wantTool)
	}
	if got := rankedFiles(e.searchJSON("widget", "text", 1)); !reflect.DeepEqual(got, wantProse[:1]) {
		t.Errorf("--limit 1 must return the strongest match, got %v", got)
	}

	// Candidate cap: 230 matches weaker than weak.jsonl push the prose match
	// count past the unfiltered 200-candidate window. The strongest documents
	// must still lead instead of falling off the end of the window.
	longFiller := strings.Repeat("pad alpha beta gamma delta ", 80)
	for i := 0; i < 230; i++ {
		e.writeRecord("weak-many.jsonl", fmt.Sprintf("weak-many-%03d", i), "weak-many", 25, fmt.Sprintf("widget once in a very long note %d ", i)+longFiller, i > 0)
	}
	for i := 0; i < 300; i++ {
		e.writeRecord("nonmatch-many.jsonl", fmt.Sprintf("nonmatch-many-%03d", i), "nonmatch-many", 26, fmt.Sprintf("nonmatching note %d about the weather and the deploy schedule with nothing else here", i), i > 0)
	}
	e.run("status", "--json")

	capped := e.searchJSON("widget", "", 300)
	if len(capped) != 200 {
		t.Fatalf("unfiltered candidate window returned %d results, want 200", len(capped))
	}
	if got := rankedFiles(capped)[:3]; !reflect.DeepEqual(got, wantProse) {
		t.Errorf("unfiltered top three past the candidate cap = %v, want %v", got, wantProse)
	}
	full := e.searchJSON("widget", "text", 300)
	if len(full) != 233 {
		t.Fatalf("--content-type text returned %d results, want 233", len(full))
	}
	if got := rankedFiles(full)[:3]; !reflect.DeepEqual(got, wantProse) {
		t.Errorf("--content-type text top three = %v, want %v", got, wantProse)
	}
	if got := rankedFiles(e.searchJSON("widget", "text", 10)); !reflect.DeepEqual(got[:3], wantProse) {
		t.Errorf("default-sized page must lead with the strongest matches, got %v", got)
	}
	if repeat := e.searchJSON("widget", "", 300); !reflect.DeepEqual(repeat, capped) {
		t.Errorf("repeat unfiltered query changed order across the candidate refill loop")
	}
}

// rankedFiles returns fixture file names in result order (queryEchoFiles sorts).
func rankedFiles(results []queryEchoResult) []string {
	files := make([]string, 0, len(results))
	for _, result := range results {
		files = append(files, filepath.Base(result.FilePath))
	}
	return files
}

func (e *queryEchoE2E) searchScores(query, contentType string, limit int) []float64 {
	e.t.Helper()
	results := e.searchJSON(query, contentType, limit)
	scores := make([]float64, len(results))
	for i, result := range results {
		scores[i] = result.Score
	}
	return scores
}

func assertAscendingNonPositiveScores(t *testing.T, label string, scores []float64) {
	t.Helper()
	for i, score := range scores {
		if score > 0 {
			t.Errorf("%s result %d score %.4f > 0; bm25 scores are never positive", label, i+1, score)
		}
		if i > 0 && score < scores[i-1] {
			t.Errorf("%s result %d score %.4f is better than result %d score %.4f; results must be best-first", label, i+1, score, i, scores[i-1])
		}
	}
}
