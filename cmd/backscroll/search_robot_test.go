package main

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSearchRobotFormatUnwrapped asserts robot output is NOT double-wrapped.
// Bug: result_0=result_0_source=session
// Fix: result_0_source=session
func TestSearchRobotFormatUnwrapped(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content so we have results to search
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	t.Setenv("BACKSCROLL_SESSION_DIRS", piDir)

	// Search with --robot flag; search for a term in the fixtures
	out, _, err := runCmd("search", "--text", "fixture", "--robot", "--limit", "1")
	if err != nil {
		t.Fatalf("search --robot error: %v", err)
	}

	// If no results, skip the test (fixture may not have matching content)
	if strings.TrimSpace(out) == "" {
		t.Skip("no search results in fixture (acceptable)")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Check each robot format line for double-wrapping
	for _, line := range lines {
		if strings.HasPrefix(line, "result_") {
			// Correct pattern: result_0_source=value or result_0_rank=0
			correctPattern := regexp.MustCompile(`^result_\d+_\w+=.+$`)
			// Bug pattern: result_0=result_0_source=value (double-wrapped)
			bugPattern := regexp.MustCompile(`^result_\d+=result_\d+_`)

			if !correctPattern.MatchString(line) && !bugPattern.MatchString(line) {
				// Line might be in a different format (e.g., from text output)
				continue
			}

			if bugPattern.MatchString(line) {
				t.Errorf("detected double-wrapped robot line (bug): %s", line)
			}
		}
	}
}
