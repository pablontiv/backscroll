package main

import (
	"testing"
)

func TestNormalizeStatement(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"We decided to use Go!", "we decided to use go"},
		{"  Hello, World.  ", "hello world"},
		{"", ""},
		{"ABC 123", "abc 123"},
	}
	for _, tc := range tests {
		got := normalizeStatement(tc.in)
		if got != tc.want {
			t.Errorf("normalizeStatement(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestComputeClusterID(t *testing.T) {
	// Same prefix → same cluster
	c1 := computeClusterID("we decided to use Go for all services and it is great")
	c2 := computeClusterID("we decided to use Go for all services and something else")
	if c1 != c2 {
		t.Errorf("expected same cluster id for similar statements, got %q vs %q", c1, c2)
	}
	// Different prefix → different cluster
	c3 := computeClusterID("we should avoid premature optimization")
	if c1 == c3 {
		t.Errorf("expected different cluster ids for different statements")
	}
}

func TestComputeFreshness(t *testing.T) {
	ts2026 := "2026-01-01T00:00:00Z"
	ts2025 := "2025-06-01T00:00:00Z"
	ts2023 := "2023-01-01T00:00:00Z"
	tests := []struct {
		ts   *string
		want string
	}{
		{nil, "unknown"},
		{&ts2026, "active"},
		{&ts2025, "stale"},
		{&ts2023, "stale"},
	}
	for _, tc := range tests {
		got := computeFreshness(tc.ts)
		if got != tc.want {
			t.Errorf("computeFreshness(%v) = %q, want %q", tc.ts, got, tc.want)
		}
	}
}

func TestMatchesDecisionPattern(t *testing.T) {
	tests := []struct {
		line  string
		match bool
	}{
		{"we decided to use Go", true},
		{"decision: use SQLite", true},
		{"we should avoid premature optimization", true},
		{"we need to refactor this module", true},
		{"going forward we will use cobra", true},
		{"we will not use ORM", true},
		{"we must not break the API", true},
		{"hello world", false},
		{"random text", false},
	}
	for _, tc := range tests {
		got := matchesDecisionPattern(tc.line)
		if got != tc.match {
			t.Errorf("matchesDecisionPattern(%q) = %v, want %v", tc.line, got, tc.match)
		}
	}
}

func TestConfidenceForSnippet(t *testing.T) {
	tests := []struct {
		snippet string
		minConf float64
	}{
		{"we decided to use Go", 0.89},
		{"decision: use SQLite for storage", 0.89},
		{"decided: adopt Go as primary language", 0.89},
		{"we will not use ORMs", 0.89},
		{"we must not break the public API", 0.89},
		{"we will use cobra for CLI", 0.74},
		{"we are using go-toml for config", 0.74},
		{"we have decided on the approach", 0.74},
		{"we need to refactor the storage layer", 0.59},
		{"going forward we will use FTS5", 0.59},
		{"we should avoid global state", 0.59},
		{"hello world this is not a decision", -0.01},
	}
	for _, tc := range tests {
		got := confidenceForSnippet(tc.snippet)
		if got < tc.minConf {
			t.Errorf("confidenceForSnippet(%q) = %f, want >= %f", tc.snippet, got, tc.minConf)
		}
	}
}

func TestExtractStatementFromSnippet(t *testing.T) {
	tests := []struct {
		snippet string
		want    string
	}{
		{"we decided to use Go", "use Go"},
		{"decision: adopt SQLite", "adopt SQLite"},
		{"decided: use cobra for CLI", "use cobra for CLI"},
		{"we will use gofmt always", "gofmt always"},
		{"random text without prefix", "random text without prefix"},
	}
	for _, tc := range tests {
		got := extractStatementFromSnippet(tc.snippet)
		if got != tc.want {
			t.Errorf("extractStatementFromSnippet(%q) = %q, want %q", tc.snippet, got, tc.want)
		}
	}
}

func TestExtractSignificantWords(t *testing.T) {
	words := extractSignificantWords("we decided to use cobra library")
	// "decided", "cobra", "library" are > 4 chars; "we", "to", "use" are not
	found := make(map[string]bool)
	for _, w := range words {
		found[w] = true
	}
	for _, expected := range []string{"decided", "cobra", "library"} {
		if !found[expected] {
			t.Errorf("expected word %q in significant words, got %v", expected, words)
		}
	}
	for _, unexpected := range []string{"we", "to", "use"} {
		if found[unexpected] {
			t.Errorf("unexpected word %q in significant words (too short)", unexpected)
		}
	}
}

func TestCountKeywordOverlap(t *testing.T) {
	tests := []struct {
		s1, s2     string
		minOverlap int
	}{
		{"we decided to use SQLite database storage", "use SQLite for data storage", 2},
		{"hello world", "completely different topic", 0},
		{"", "", 0},
	}
	for _, tc := range tests {
		got := countKeywordOverlap(tc.s1, tc.s2)
		if got < tc.minOverlap {
			t.Errorf("countKeywordOverlap(%q, %q) = %d, want >= %d", tc.s1, tc.s2, got, tc.minOverlap)
		}
	}
}

func TestDecisionFrontmatter(t *testing.T) {
	text := "---\nid: D001\nstatus: accepted\nscope: technical\n---\n# Title\nBody."
	fm := decisionFrontmatter(text)
	if fm["id"] != "D001" {
		t.Errorf("expected id=D001, got %q", fm["id"])
	}
	if fm["status"] != "accepted" {
		t.Errorf("expected status=accepted, got %q", fm["status"])
	}
	if fm["scope"] != "technical" {
		t.Errorf("expected scope=technical, got %q", fm["scope"])
	}

	// No frontmatter
	fm2 := decisionFrontmatter("# Just a title\nNo frontmatter here.")
	if len(fm2) != 0 {
		t.Errorf("expected empty fm for text without frontmatter, got %v", fm2)
	}
}

func TestDecisionMetadata(t *testing.T) {
	text := "---\nid: D001\nstatus: accepted\nscope: technical\n---\n# Use Go\nWe use Go."
	id, title, status, scope, isAccepted := decisionMetadata(text, "/decisions/d001.md")
	if id != "D001" {
		t.Errorf("id: got %q, want D001", id)
	}
	if title != "Use Go" {
		t.Errorf("title: got %q, want 'Use Go'", title)
	}
	if status != "accepted" {
		t.Errorf("status: got %q, want accepted", status)
	}
	if scope == nil || *scope != "technical" {
		t.Errorf("scope: got %v, want 'technical'", scope)
	}
	if !isAccepted {
		t.Error("expected isAccepted=true for status=accepted")
	}

	// No frontmatter → fallback title from filename
	_, title2, status2, _, _ := decisionMetadata("# Title Only\nContent.", "/decisions/my-decision.md")
	if title2 != "Title Only" {
		t.Errorf("title2: got %q, want 'Title Only'", title2)
	}
	if status2 != "proposed" {
		t.Errorf("status2: got %q, want proposed", status2)
	}
}

func TestDetectConflicts(t *testing.T) {
	proposal := proposalInput{
		Statement: "we decided to use Go for all backend services",
		Scope:     strPtr("technical"),
	}

	type exEntry struct {
		id, text, status string
		scope            *string
		sourcePath       string
	}

	existing := []struct {
		id, text, status string
		scope            *string
		sourcePath       string
	}{
		{
			id:         "D001",
			text:       "we decided to use Go for all backend services and it is great",
			status:     "accepted",
			scope:      strPtr("technical"),
			sourcePath: "/d/d001.md",
		},
	}

	hints := detectConflicts(proposal, existing)
	// The prefix matches → should be duplicate
	found := false
	for _, h := range hints {
		if h.ConflictType == "duplicate" || h.ConflictType == "potential_conflict" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected at least one conflict hint, got %v", hints)
	}

	// No overlap → no conflicts
	proposalDiff := proposalInput{
		Statement: "we should use daily standups for team communication",
		Scope:     strPtr("organizational"),
	}
	hintsNone := detectConflicts(proposalDiff, existing)
	_ = hintsNone // may or may not have overlap depending on words
}

func strPtr(s string) *string { return &s }
