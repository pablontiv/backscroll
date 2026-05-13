package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestNewFormatter(t *testing.T) {
	f := NewFormatter(FormatJSON, 1000)
	if f.Format != FormatJSON {
		t.Errorf("expected format JSON, got %s", f.Format)
	}
	if f.MaxTokens != 1000 {
		t.Errorf("expected MaxTokens 1000, got %d", f.MaxTokens)
	}
}

func TestWriteResultsText(t *testing.T) {
	formatter := NewFormatter(FormatText, 0)
	results := []models.SearchResult{
		{
			Source:    "session",
			Role:      "user",
			Content:   "This is a test query",
			FilePath:  "/path/to/session.jsonl",
			SessionID: "sess-123",
			Score:     0.95,
			Tags:      []string{"debugging", "feature"},
			Rank:      1,
		},
	}

	var buf bytes.Buffer
	if err := formatter.WriteResults(&buf, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Check that key information is present
	if !strings.Contains(output, "session") {
		t.Error("expected 'session' in output")
	}
	if !strings.Contains(output, "user") {
		t.Error("expected 'user' in output")
	}
	if !strings.Contains(output, "This is a test query") {
		t.Error("expected content in output")
	}
	if !strings.Contains(output, "/path/to/session.jsonl") {
		t.Error("expected filepath in output")
	}
	if !strings.Contains(output, "sess-123") {
		t.Error("expected session ID in output")
	}
	if !strings.Contains(output, "0.95") {
		t.Error("expected score in output")
	}
}

func TestWriteResultsJSON(t *testing.T) {
	formatter := NewFormatter(FormatJSON, 0)
	results := []models.SearchResult{
		{
			Source:    "session",
			Role:      "user",
			Content:   "Test content",
			FilePath:  "/path/to/file.jsonl",
			SessionID: "sess-456",
			Score:     0.87,
		},
	}

	var buf bytes.Buffer
	if err := formatter.WriteResults(&buf, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse output as JSON
	var parsed []models.SearchResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if len(parsed) != 1 {
		t.Errorf("expected 1 result, got %d", len(parsed))
	}
	if parsed[0].SessionID != "sess-456" {
		t.Errorf("expected session ID sess-456, got %s", parsed[0].SessionID)
	}
}

func TestWriteResultsRobot(t *testing.T) {
	formatter := NewFormatter(FormatRobot, 0)
	results := []models.SearchResult{
		{
			Source:      "plan",
			Role:        "assistant",
			Content:     "Plan content",
			FilePath:    "/path/to/plan.md",
			ProjectPath: "/home/user/project",
			Score:       0.75,
			Tags:        []string{"refactoring"},
			Rank:        1,
		},
	}

	var buf bytes.Buffer
	if err := formatter.WriteResults(&buf, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Check robot format key=value pairs
	if !strings.Contains(output, "result_0_source=plan") {
		t.Error("expected 'result_0_source=plan' in output")
	}
	if !strings.Contains(output, "result_0_role=assistant") {
		t.Error("expected 'result_0_role=assistant' in output")
	}
	if !strings.Contains(output, "result_0_content=Plan content") {
		t.Error("expected content in robot output")
	}
	if !strings.Contains(output, "result_0_project=/home/user/project") {
		t.Error("expected project in robot output")
	}
	if !strings.Contains(output, "result_0_tags=refactoring") {
		t.Error("expected tags in robot output")
	}
}

func TestTokenLimiting(t *testing.T) {
	// Create multiple results with significant content
	results := make([]models.SearchResult, 5)
	for i := 0; i < 5; i++ {
		results[i] = models.SearchResult{
			Source:  "session",
			Role:    "user",
			Content: strings.Repeat("word ", 50), // ~65 tokens per result (50 * 1.3)
			Score:   0.9,
			Rank:    i + 1,
		}
	}

	// Limit to ~100 tokens (should get about 1-2 results)
	formatter := NewFormatter(FormatText, 100)

	var buf bytes.Buffer
	if err := formatter.WriteResults(&buf, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Count how many "Rank:" appear in output (proxy for number of results)
	resultCount := strings.Count(output, "Rank:")
	if resultCount == 0 {
		t.Error("expected at least one result in limited output")
	}
	if resultCount > 3 {
		t.Errorf("expected token limiting to keep results low, got %d results", resultCount)
	}
}

func TestNoTokenLimit(t *testing.T) {
	// Create multiple results
	results := make([]models.SearchResult, 5)
	for i := 0; i < 5; i++ {
		results[i] = models.SearchResult{
			Source:  "session",
			Role:    "user",
			Content: "Test content",
			Score:   0.9,
			Rank:    i + 1,
		}
	}

	// No token limit (MaxTokens = 0)
	formatter := NewFormatter(FormatText, 0)

	var buf bytes.Buffer
	if err := formatter.WriteResults(&buf, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// All results should be included
	resultCount := strings.Count(output, "Rank:")
	if resultCount != 5 {
		t.Errorf("expected 5 results with no token limit, got %d", resultCount)
	}
}

func TestWriteJSON(t *testing.T) {
	formatter := NewFormatter(FormatText, 0)

	data := map[string]interface{}{
		"name":  "test",
		"count": 42,
	}

	var buf bytes.Buffer
	if err := formatter.WriteJSON(&buf, data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["name"] != "test" {
		t.Errorf("expected name 'test', got %v", parsed["name"])
	}
	if parsed["count"] != float64(42) {
		t.Errorf("expected count 42, got %v", parsed["count"])
	}
}

func TestEstimateTokens(t *testing.T) {
	formatter := NewFormatter(FormatText, 0)

	result := models.SearchResult{
		Content:   "one two three four five", // 5 words
		FilePath:  "/path/to/file",           // 3 words
		SessionID: "session-123",             // 1 word
		Tags:      []string{"tag1", "tag2"},  // 2 tags
	}

	tokens := formatter.estimateTokens(result)
	// Expected: (5 + 3 + 1 + 2) * 1.3 ≈ 13.65 ≈ 13, but min 10
	if tokens < 10 {
		t.Errorf("expected tokens >= 10, got %d", tokens)
	}
}

func TestMultipleResults(t *testing.T) {
	formatter := NewFormatter(FormatText, 0)

	now := time.Now()
	results := []models.SearchResult{
		{
			Source:      "session",
			Role:        "user",
			Content:     "First result",
			FilePath:    "/path/1",
			Timestamp:   now,
			SessionID:   "sess-1",
			ProjectPath: "/proj/1",
			Score:       0.95,
			Tags:        []string{"tag1"},
			ContentType: "text",
			Rank:        1,
		},
		{
			Source:      "plan",
			Role:        "assistant",
			Content:     "Second result",
			FilePath:    "/path/2",
			Timestamp:   now.Add(-1 * time.Hour),
			SessionID:   "sess-2",
			ProjectPath: "/proj/2",
			Score:       0.87,
			Tags:        []string{"tag2", "tag3"},
			ContentType: "code",
			Rank:        2,
		},
	}

	var buf bytes.Buffer
	if err := formatter.WriteResults(&buf, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Verify both results are present
	if !strings.Contains(output, "First result") {
		t.Error("expected first result content")
	}
	if !strings.Contains(output, "Second result") {
		t.Error("expected second result content")
	}
	if !strings.Contains(output, "sess-1") {
		t.Error("expected first session ID")
	}
	if !strings.Contains(output, "sess-2") {
		t.Error("expected second session ID")
	}
}
