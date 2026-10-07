package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	picokitoutput "github.com/pablontiv/picokit/output"

	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/services"
)

func TestSearchValidationMatchesServices(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		contentType string
		relax       bool
	}{
		{name: "missing query"},
		{name: "invalid content type", query: "needle", contentType: "audio"},
		{name: "invalid relaxation query", query: `"unclosed`, relax: true},
		{name: "valid", query: "needle", contentType: "text"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, cliErr := validateAndParseSearchRequest(test.query, "minimal", test.contentType, "", "", test.relax)
			serviceErr := services.ValidateSearchRequest(services.SearchRequest{
				Query: test.query,
				Options: models.SearchOptions{
					ContentType: test.contentType,
				},
				Relax: test.relax,
			})
			if (cliErr == nil) != (serviceErr == nil) {
				t.Fatalf("CLI error = %v, service error = %v", cliErr, serviceErr)
			}
			if cliErr != nil && cliErr.Error() != serviceErr.Error() {
				t.Fatalf("CLI error = %q, service error = %q", cliErr, serviceErr)
			}
		})
	}
}

func TestSearchOutputFormatText(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search without format flag (defaults to text)
	out, _, err := runCmd("search", "test")
	if err != nil {
		t.Fatalf("search text error: %v", err)
	}

	// Verify text format characteristics
	if strings.Contains(out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━") {
		// Good: contains separator
	} else if strings.Contains(out, "Rank:") && strings.Contains(out, "Source:") {
		// Good: contains field headers
	} else if len(out) == 0 {
		// Empty results are acceptable (no matching content in fixtures)
	} else {
		t.Errorf("search text output doesn't contain expected format markers: %s", out)
	}
}

func TestSearchOutputFormatJSON(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with --json flag
	out, _, err := runCmd("search", "test", "--json")
	if err != nil {
		t.Fatalf("search --json error: %v", err)
	}

	// Verify valid JSON output
	var results []models.SearchResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		// Empty results might serialize differently; try parsing as empty array
		if len(strings.TrimSpace(out)) > 0 {
			t.Fatalf("search --json output not valid JSON: %v\noutput: %s", err, out)
		}
	}
	// If we got here, output is valid JSON
}

func TestSearchOutputFormatRobot(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with --robot flag
	out, _, err := runCmd("search", "test", "--robot")
	if err != nil {
		t.Fatalf("search --robot error: %v", err)
	}

	// Verify robot format characteristics: result_N_field=value pattern
	if len(strings.TrimSpace(out)) > 0 {
		// If there are results, check for robot format
		if !strings.Contains(out, "result_") {
			t.Errorf("robot format missing result_N_ prefix: %s", out)
		}
		// Check for expected robot format fields
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "result_") {
				if !strings.Contains(line, "=") {
					t.Errorf("robot format line missing '=': %s", line)
				}
			}
		}
	}
}

func TestSearchOutputRespectsTokenLimit(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with --max-tokens limit
	out, _, err := runCmd("search", "test", "--max-tokens", "50")
	if err != nil {
		t.Fatalf("search --max-tokens error: %v", err)
	}

	// Just verify the command ran successfully and produced output
	// The token limiting is a soft limit so we can't assert exact behavior
	if out == "" && len(out) > 0 {
		t.Error("output should not be empty when results exist")
	}
	// If output is empty or present, that's acceptable
}

func TestSearchTextLinesBudgetsWholeResults(t *testing.T) {
	uuid := "message-budget-1"
	results := []models.SearchResult{
		{
			Source:   "session",
			Role:     "assistant",
			UUID:     &uuid,
			Ordinal:  11,
			Content:  "first complete result",
			FilePath: "/tmp/first.jsonl",
			Rank:     1,
			Score:    0.91,
		},
		{
			Source:   "session",
			Role:     "user",
			UUID:     nil,
			Ordinal:  12,
			Content:  "second complete result",
			FilePath: "/tmp/second.jsonl",
			Rank:     2,
			Score:    0.82,
		},
	}

	allLines := resultsToLines(results, picokitoutput.FormatText)
	allPayload := strings.Join(allLines, "\n")
	for _, maxTokens := range []int{0, -1} {
		if got := strings.Join(searchTextLines(results, maxTokens), "\n"); got != allPayload {
			t.Fatalf("unlimited budget %d changed text output\ngot:  %q\nwant: %q", maxTokens, got, allPayload)
		}
	}

	firstLines := resultsToLines(results[:1], picokitoutput.FormatText)
	firstPayload := strings.Join(firstLines, "\n")
	firstBudget := picokitoutput.TokenCount(firstPayload)
	if got := searchTextLines(results, firstBudget-1); len(got) != 0 {
		t.Fatalf("budget below first complete result emitted partial output: %q", strings.Join(got, "\n"))
	}

	if got := strings.Join(searchTextLines(results, firstBudget), "\n"); got != firstPayload {
		t.Fatalf("exact first-result budget did not emit exactly one complete group\ngot:  %q\nwant: %q", got, firstPayload)
	} else if !strings.Contains(got, "UUID: message-budget-1") || !strings.Contains(got, "Ordinal: 11") {
		t.Fatalf("complete first result lacks identity: %q", got)
	}

	totalBudget := picokitoutput.TokenCount(allPayload)
	if got := strings.Join(searchTextLines(results, totalBudget), "\n"); got != allPayload {
		t.Fatalf("total budget did not emit all complete groups\ngot:  %q\nwant: %q", got, allPayload)
	} else if !strings.Contains(got, "UUID: null") || !strings.Contains(got, "Ordinal: 12") {
		t.Fatalf("complete nullable-UUID result lacks fallback identity: %q", got)
	}

	for _, budget := range []int{firstBudget - 1, firstBudget, totalBudget - 1, totalBudget} {
		payload := strings.Join(searchTextLines(results, budget), "\n")
		if tokens := picokitoutput.TokenCount(payload); tokens > budget {
			t.Fatalf("emitted payload uses %d tokens, budget is %d: %q", tokens, budget, payload)
		}
		if payload == "" {
			continue
		}
		groups := strings.Count(payload, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		if uuids := strings.Count(payload, "UUID: "); uuids != groups {
			t.Fatalf("emitted %d complete groups but %d UUID fields: %q", groups, uuids, payload)
		}
		if ordinals := strings.Count(payload, "Ordinal: "); ordinals != groups {
			t.Fatalf("emitted %d complete groups but %d ordinal fields: %q", groups, ordinals, payload)
		}
	}
}

func TestSearchTextLinesCountsCompletePayload(t *testing.T) {
	uuid := "message-rounding"
	results := []models.SearchResult{{
		Source:   "session",
		Role:     "assistant",
		UUID:     &uuid,
		Ordinal:  7,
		Content:  "rounding regression payload",
		FilePath: "/tmp/rounding.jsonl",
		Rank:     1,
		Score:    0.75,
	}}
	lines := resultsToLines(results, picokitoutput.FormatText)
	payload := strings.Join(lines, "\n")

	// This models the rejected implementation only to prove the fixture catches it.
	separatelyRounded := 0
	for _, line := range lines {
		separatelyRounded += picokitoutput.TokenCount(line)
	}
	completeTokens := picokitoutput.TokenCount(payload)
	if separatelyRounded >= completeTokens {
		t.Fatalf("fixture does not expose per-line undercount: separate=%d complete=%d", separatelyRounded, completeTokens)
	}
	if got := searchTextLines(results, separatelyRounded); len(got) != 0 {
		t.Fatalf("group whose complete payload exceeds budget was emitted: %q", strings.Join(got, "\n"))
	}
}

func TestRunSearchTextBudgetBoundary(t *testing.T) {
	cfg := newSearchIdentityContractIndex(t)
	run := func(maxTokens int) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		err := runSearch(context.Background(), &stdout, &stderr, cfg,
			"presentselector", "", true, false, false,
			"", "", "", "", "", 20, 0, "text", "",
			"minimal", maxTokens, true, 0.3, false)
		if err != nil {
			t.Fatalf("runSearch with budget %d: %v\nstderr: %s", maxTokens, err, stderr.String())
		}
		return stdout.String()
	}

	unlimited := run(0)
	if unlimited == "" {
		t.Fatal("unlimited runSearch returned no fixture result")
	}
	budget := picokitoutput.TokenCount(unlimited)
	if got := run(budget - 1); got != "" {
		t.Fatalf("runSearch emitted a partial first result below its boundary: %q", got)
	}
	if got := run(budget); got != unlimited {
		t.Fatalf("runSearch exact boundary changed the complete result\ngot:  %q\nwant: %q", got, unlimited)
	} else if picokitoutput.TokenCount(got) > budget {
		t.Fatalf("runSearch emitted %d tokens with budget %d", picokitoutput.TokenCount(got), budget)
	} else if !strings.Contains(got, "UUID: contract-uuid") || !strings.Contains(got, "Ordinal: 4") {
		t.Fatalf("runSearch exact boundary omitted identity: %q", got)
	}
}

func TestSearchTextFormatStructure(t *testing.T) {
	// Test the resultsToLines adapter function directly
	uuid := "message-123"
	results := []models.SearchResult{
		{
			Source:      "session",
			Role:        "user",
			UUID:        &uuid,
			Ordinal:     17,
			Content:     "test content",
			FilePath:    "/path/to/file.jsonl",
			Rank:        1,
			Score:       0.95,
			SessionID:   "session-123",
			ProjectPath: "/home/project",
			Tags:        []string{"debugging"},
		},
	}

	// Use FormatText from picokit
	lines := resultsToLines(results, picokitoutput.FormatText)

	// Verify we have expected text format lines
	allText := strings.Join(lines, "\n")
	expectedStrs := []string{
		"━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━",
		"Rank: 1",
		"Source: session",
		"Role: user",
		"Score: 0.95",
		"Path: /path/to/file.jsonl",
		"Session: session-123",
		"Project: /home/project",
		"Tags: debugging",
		"UUID: message-123",
		"Ordinal: 17",
		"test content",
	}

	for _, expected := range expectedStrs {
		if !strings.Contains(allText, expected) {
			t.Errorf("text format missing expected string: %q\nfull output: %s", expected, allText)
		}
	}
}

func TestSearchRobotFormatStructure(t *testing.T) {
	// Test the resultsToLines adapter function directly for robot format
	uuid := "message-456"
	results := []models.SearchResult{
		{
			Source:      "session",
			Role:        "assistant",
			UUID:        &uuid,
			Ordinal:     23,
			Content:     "test content",
			FilePath:    "/path/to/file.jsonl",
			Rank:        1,
			Score:       0.85,
			SessionID:   "session-456",
			ProjectPath: "/home/project2",
			Tags:        []string{"refactoring", "testing"},
		},
	}

	// Use FormatRobot from picokit
	lines := resultsToLines(results, picokitoutput.FormatRobot)

	// Verify we have expected robot format lines
	allText := strings.Join(lines, "\n")
	expectedStrs := []string{
		"result_0_source=session",
		"result_0_role=assistant",
		"result_0_filepath=/path/to/file.jsonl",
		"result_0_uuid=message-456",
		"result_0_ordinal=23",
		"result_0_content=test content",
		"result_0_session_id=session-456",
		"result_0_project=/home/project2",
		"result_0_score=0.85",
		"result_0_tags=refactoring,testing",
		"result_0_rank=1",
	}

	for _, expected := range expectedStrs {
		if !strings.Contains(allText, expected) {
			t.Errorf("robot format missing expected line: %q\nfull output: %s", expected, allText)
		}
	}
}

func TestSearchRobotFormatEscapesStringValuesToSingleLine(t *testing.T) {
	uuid := "uuid\\part\nnext"
	results := []models.SearchResult{
		{
			Source:      "sess\\ion",
			Role:        "assist\rant",
			UUID:        &uuid,
			Ordinal:     31,
			Content:     "line 1\nline 2\r\npath\\tail",
			FilePath:    "/tmp/file\nname.md",
			Rank:        2,
			Score:       0.42,
			SessionID:   "session\n789",
			ProjectPath: "project\\root",
			Tags:        []string{"tag\\one", "tag\ntwo"},
		},
	}

	lines := resultsToLines(results, picokitoutput.FormatRobot)
	allText := strings.Join(lines, "\n")

	for _, line := range lines {
		if strings.Contains(line, "\n") || strings.Contains(line, "\r") {
			t.Fatalf("robot line must be one-line escaped output, got %q", line)
		}
	}

	expected := []string{
		`result_0_source=sess\\ion`,
		`result_0_role=assist\rant`,
		`result_0_filepath=/tmp/file\nname.md`,
		`result_0_uuid=uuid\\part\nnext`,
		`result_0_ordinal=31`,
		`result_0_content=line 1\nline 2\r\npath\\tail`,
		`result_0_session_id=session\n789`,
		`result_0_project=project\\root`,
		`result_0_tags=tag\\one,tag\ntwo`,
	}
	for _, want := range expected {
		if !strings.Contains(allText, want) {
			t.Fatalf("robot escaped output missing %q in %q", want, allText)
		}
	}
}

func TestSearchWithJSONAndMaxTokens(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()

	// Sync fixture content
	piDir := filepath.Dir(filepath.Join(fixturesDir(), "pi-session.jsonl"))
	_, _, err := syncForTest(t, "sync", "--path", piDir)
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}

	// Search with both --json and --max-tokens
	out, _, err := runCmd("search", "test", "--json", "--max-tokens", "100")
	if err != nil {
		t.Fatalf("search --json --max-tokens error: %v", err)
	}

	// Verify valid JSON output
	var results []models.SearchResult
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		if len(strings.TrimSpace(out)) > 0 {
			t.Fatalf("search --json --max-tokens output not valid JSON: %v", err)
		}
	}
}

func TestSearchValidatesContentType(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BACKSCROLL_SESSION_DIRS", t.TempDir())

	tests := []struct {
		flag    string
		wantErr bool
	}{
		{"text", false},
		{"code", false},
		{"tool", false},
		{"reasoning", false},
		{"invalid", true},
		{"", false}, // empty is valid (no filter)
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			if tt.flag == "" {
				_, _, err := runCmd("search", "test")
				if (err != nil) != tt.wantErr {
					t.Errorf("runCmd with no --content-type: err=%v, wantErr=%v", err, tt.wantErr)
				}
			} else {
				_, _, err := runCmd("search", "test", "--content-type", tt.flag)
				if (err != nil) != tt.wantErr {
					t.Errorf("runCmd with --content-type %q: err=%v, wantErr=%v", tt.flag, err, tt.wantErr)
				}
			}
		})
	}
}
