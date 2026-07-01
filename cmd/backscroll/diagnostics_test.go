package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteSearchHints_ProjectScoped(t *testing.T) {
	var buf bytes.Buffer
	writeSearchHints(&buf, false, false)
	out := buf.String()
	if !strings.Contains(out, "--all-projects") {
		t.Errorf("project-scoped hint should suggest --all-projects, got:\n%s", out)
	}
	if !strings.Contains(out, "--content-type tool") {
		t.Errorf("non-tool query should suggest --content-type tool, got:\n%s", out)
	}
	if !strings.Contains(out, "backscroll status") {
		t.Errorf("hint should mention backscroll status, got:\n%s", out)
	}
}

func TestWriteSearchHints_AllProjectsAndToolScoped(t *testing.T) {
	var buf bytes.Buffer
	writeSearchHints(&buf, true, true)
	out := buf.String()
	if strings.Contains(out, "--all-projects") {
		t.Errorf("already all-projects: should not suggest --all-projects, got:\n%s", out)
	}
	if strings.Contains(out, "--content-type tool") {
		t.Errorf("already tool-scoped: should not suggest --content-type tool, got:\n%s", out)
	}
	if !strings.Contains(out, "backscroll status") {
		t.Errorf("status hint should always appear, got:\n%s", out)
	}
}

func TestSearchZeroResultHintsToStderr(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Initialize the database by running validate
	_, _, err := runCmd("validate", "--indexed-only")
	if err != nil {
		// validate may fail if no sessions exist, which is fine
	}

	// A query that cannot match anything; --json keeps stdout a clean empty array.
	out, stderr, err := runCmd("search", "zzqqxx_no_such_token_zzqqxx", "--json", "--indexed-only")
	if err != nil {
		t.Fatalf("search error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "no results") {
		t.Errorf("expected zero-result hint on stderr, got: %q", stderr)
	}
	if strings.Contains(out, "•") || strings.Contains(out, "suggestions") {
		t.Errorf("hints must not leak into stdout, got: %q", out)
	}
}

func TestWarnShortToolQuery(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		query       string
		wantWarn    bool
	}{
		{"short tool query warns", "tool", "go", true},
		{"short tool query with spaces warns", "tool", " cd ", true},
		{"three-char tool query is fine", "tool", "git", false},
		{"short query but not tool-scoped", "", "go", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			warnShortToolQuery(&buf, tc.contentType, tc.query)
			got := strings.Contains(buf.String(), "under 3 characters")
			if got != tc.wantWarn {
				t.Errorf("warn=%v want=%v (out=%q)", got, tc.wantWarn, buf.String())
			}
		})
	}
}

func TestSearchShortToolQueryWarnsToStderr(t *testing.T) {
	out, stderr, err := runCmd("search", "go", "--content-type", "tool", "--json", "--indexed-only")
	if err != nil {
		t.Fatalf("search error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "under 3 characters") {
		t.Errorf("expected short-query warning on stderr, got: %q", stderr)
	}
	if strings.Contains(out, "under 3 characters") {
		t.Errorf("warning must not leak into stdout, got: %q", out)
	}
}
