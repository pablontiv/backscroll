package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
	picokitoutput "github.com/pablontiv/picokit/output"
)

func TestSearchRelaxationBudgetKeepsProvenanceWithEachResult(t *testing.T) {
	results := []storage.SearchResult{
		{SourcePath: "/a", Text: "alpha beta", Snippet: "alpha beta", MatchStage: "drop-terms", DroppedTerms: []string{"common", "line\nwith\\slash"}},
		{SourcePath: "/b", Text: "alpha beta", Snippet: "alpha beta", MatchStage: "drop-terms", DroppedTerms: []string{"common", "line\nwith\\slash"}},
	}
	for _, fields := range []string{"minimal", "full"} {
		for budget := 1; budget < 100; budget++ {
			lines := searchRobotLines(results, fields, budget)
			payload := strings.Join(lines, "\n")
			if picokitoutput.TokenCount(payload) > budget {
				t.Fatalf("payload exceeds %d tokens: %s", budget, payload)
			}
			count := strings.Count(payload, "_filepath=")
			if count != strings.Count(payload, "_match_stage=drop-terms") || count != strings.Count(payload, "_dropped_terms=") {
				t.Fatalf("partial result/provenance at budget %d: %s", budget, payload)
			}
			if strings.Contains(payload, "line\nwith") {
				t.Fatalf("unescaped provenance split a robot line: %s", payload)
			}
		}
	}
}

func TestSearchRelaxationInvalidSyntaxPrecedesStartup(t *testing.T) {
	testEnv(t)
	for _, query := range []string{`"unclosed`, "+", `""`, "+ word"} {
		path := filepath.Join(t.TempDir(), "must-not-exist.db")
		t.Setenv("BACKSCROLL_DATABASE_PATH", path)
		out, _, err := runCmd("search", "--text", query, "--relax", "--robot")
		if err == nil || !strings.Contains(err.Error(), "invalid --relax query") || out != "" {
			t.Fatalf("expected pure validation failure: %v %q", err, out)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid request created database: %v", err)
		}
		if _, err := os.Stat(path + ".startup-sync.lock"); !os.IsNotExist(err) {
			t.Fatalf("invalid request created startup sidecar: %v", err)
		}
	}
}
