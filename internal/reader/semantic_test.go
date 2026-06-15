package reader

import (
	"strings"
	"testing"
)

func TestReadSemanticTailIncludesPiToolCallArguments(t *testing.T) {
	rows, err := ReadSemanticTail("../../tests/fixtures/pi-session.jsonl", 0)
	if err != nil {
		t.Fatalf("ReadSemanticTail: %v", err)
	}

	for _, row := range rows {
		if row.Kind == "tool_use" && strings.Contains(row.Content, "name=read") {
			if !strings.Contains(row.Content, `"path":"secret"`) {
				t.Fatalf("tool_use row missing Pi arguments payload: %#v", row)
			}
			return
		}
	}
	t.Fatalf("missing Pi tool_use row in %#v", rows)
}

func TestReadSemanticTailClassifiesPiToolResultRole(t *testing.T) {
	rows, err := ReadSemanticTail("../../tests/fixtures/pi-session.jsonl", 0)
	if err != nil {
		t.Fatalf("ReadSemanticTail: %v", err)
	}

	for _, row := range rows {
		if row.Role == "toolResult" {
			if row.Kind != "tool_result" {
				t.Fatalf("toolResult role row kind = %q, want tool_result: %#v", row.Kind, row)
			}
			if !strings.Contains(row.Content, "tool result should not index") {
				t.Fatalf("toolResult role row missing content snippet: %#v", row)
			}
			return
		}
	}
	t.Fatalf("missing Pi toolResult role row in %#v", rows)
}

func TestSemanticTextRow(t *testing.T) {
	// Test that semanticTextRow creates a row when text is present
	base := SemanticRow{Path: "test.jsonl", Line: 1, Timestamp: "2024-01-01T00:00:00Z", Role: "user"}
	rows := semanticTextRow(base, "hello world", "text")
	if len(rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Content != "hello world" {
		t.Errorf("expected content 'hello world', got %q", rows[0].Content)
	}

	// Test that semanticTextRow returns nil for empty text
	rows = semanticTextRow(base, "", "text")
	if len(rows) != 0 {
		t.Errorf("expected 0 rows for empty text, got %d", len(rows))
	}

	// Test truncation of long text
	longText := "word " + strings.Repeat("x", 600)
	rows = semanticTextRow(base, longText, "text")
	if len(rows) != 1 {
		t.Errorf("expected 1 row for long text, got %d", len(rows))
	}
	if !strings.HasSuffix(rows[0].Content, "…") {
		t.Errorf("expected truncated content to end with …, got %q", rows[0].Content)
	}
	// The length after Fields() normalization varies, just check it ends with …
	// and is roughly the right size (500 + 3 for "…" encoding)
	if len(rows[0].Content) < 500 {
		t.Errorf("expected long truncated content, got %d", len(rows[0].Content))
	}
}

func TestFirstNonEmpty(t *testing.T) {
	// Test with all non-empty
	result := firstNonEmpty("a", "b", "c")
	if result != "a" {
		t.Errorf("expected 'a', got %q", result)
	}

	// Test with empty first value
	result = firstNonEmpty("", "b", "c")
	if result != "b" {
		t.Errorf("expected 'b', got %q", result)
	}

	// Test with all empty
	result = firstNonEmpty("", "", "")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}

	// Test with single value
	result = firstNonEmpty("value")
	if result != "value" {
		t.Errorf("expected 'value', got %q", result)
	}
}

func TestReadSemanticTailWithLimit(t *testing.T) {
	// Test with non-zero tail limit (ringbuffer logic)
	rows, err := ReadSemanticTail("../../tests/fixtures/pi-session.jsonl", 2)
	if err != nil {
		t.Fatalf("ReadSemanticTail with tail=2: %v", err)
	}
	// Should return last 2 rows
	if len(rows) > 2 {
		t.Errorf("expected at most 2 rows, got %d", len(rows))
	}
}

func TestJsonField(t *testing.T) {
	// Test finding nested JSON values
	m := map[string]any{
		"input": map[string]any{
			"arguments": map[string]any{
				"toolCall": "test_value",
			},
		},
	}

	result := jsonField(m, "input", "arguments", "toolCall")
	if !strings.Contains(result, "test_value") {
		t.Errorf("expected JSON containing 'test_value', got %q", result)
	}

	// Test with missing key
	result = jsonField(m, "missing", "key")
	if result != "" {
		t.Errorf("expected empty string for missing key, got %q", result)
	}

	// Test with null value
	m2 := map[string]any{"key": nil}
	result = jsonField(m2, "key")
	if result != "" {
		t.Errorf("expected empty string for null value, got %q", result)
	}
}
