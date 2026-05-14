package input_config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeJSONL(t *testing.T, path string, records []map[string]any) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			t.Fatal(err)
		}
	}
}

func claudeDef() InputDefinition {
	return InputDefinition{
		ID:     "claude",
		Active: true,
		Decode: DecodeConfig{Format: "jsonl"},
		Record: RecordConfig{
			Selector: "$",
			IncludeWhen: []Predicate{
				{Selector: "$.type", Op: "in", Value: []any{"user", "assistant"}},
			},
			ExcludeWhen: []Predicate{
				{Selector: "$.isMeta", Op: "eq", Value: true},
			},
		},
		Map: MapConfig{
			Role:      "$.message.role",
			UUID:      "$.uuid",
			Timestamp: "$.timestamp",
			SessionID: "$.sessionId",
		},
		Content: ContentConfig{
			Selector:  "$.message.content",
			BlockText: "$.text",
			IncludeWhen: []Predicate{
				{Selector: "$.type", Op: "eq", Value: "text"},
			},
		},
		Text: TextConfig{
			Trim:      true,
			DropEmpty: true,
		},
	}
}

func TestTestFile_basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{
			"type": "user",
			"uuid": "abc-123",
			"message": map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "hello world"}},
			},
		},
		{
			"type":   "user",
			"isMeta": true, // should be excluded
			"message": map[string]any{
				"role":    "user",
				"content": "meta",
			},
		},
		{
			"type": "system-reminder", // should be excluded (type not in [user, assistant])
			"message": map[string]any{
				"role":    "system",
				"content": "noise",
			},
		},
	}
	writeJSONL(t, path, records)

	results, err := TestFile(path, claudeDef())
	if err != nil {
		t.Fatalf("TestFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %v", len(results), results)
	}
	if results[0].Role != "user" {
		t.Errorf("role = %q, want user", results[0].Role)
	}
	if results[0].UUID != "abc-123" {
		t.Errorf("uuid = %q, want abc-123", results[0].UUID)
	}
	if results[0].Content != "hello world" {
		t.Errorf("content = %q, want %q", results[0].Content, "hello world")
	}
}

func TestTestFile_dropEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{
			"type": "user",
			"message": map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "   "}}, // only whitespace
			},
		},
	}
	writeJSONL(t, path, records)

	results, err := TestFile(path, claudeDef())
	if err != nil && !errors.Is(err, ErrDropped) {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (drop_empty), got %d", len(results))
	}
}

func TestTestFile_unsupportedFormat(t *testing.T) {
	def := InputDefinition{Decode: DecodeConfig{Format: "markdown"}}
	_, err := TestFile("/any/path", def)
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestTestFile_stringContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{
			"type": "user",
			"message": map[string]any{
				"role":    "user",
				"content": "direct string content",
			},
		},
	}
	writeJSONL(t, path, records)

	def := claudeDef()
	def.Content.BlockText = ""    // no block extraction
	def.Content.IncludeWhen = nil // no block filter

	results, err := TestFile(path, def)
	if err != nil {
		t.Fatalf("TestFile: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "direct string content" {
		t.Errorf("content = %q, want %q", results[0].Content, "direct string content")
	}
}

func TestSelectString(t *testing.T) {
	record := map[string]any{"role": "user", "count": float64(3)}
	s, ok := SelectString(record, "$.role")
	if !ok || s != "user" {
		t.Errorf("SelectString role: got %q %v", s, ok)
	}
	s, ok = SelectString(record, "$.count")
	if !ok || s != "3" {
		t.Errorf("SelectString count: got %q %v", s, ok)
	}
	_, ok = SelectString(record, "$.missing")
	if ok {
		t.Error("expected false for missing field")
	}
}

func TestTraverse_arrayIndex(t *testing.T) {
	record := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	v, ok := SelectField(record, "$.items[1]")
	if !ok || v != "b" {
		t.Errorf("items[1] = %v %v, want b true", v, ok)
	}
}

func TestTraverse_outOfBounds(t *testing.T) {
	record := map[string]any{
		"items": []any{"a"},
	}
	_, ok := SelectField(record, "$.items[5]")
	if ok {
		t.Error("expected false for out-of-bounds index")
	}
}

func TestInputModeString(t *testing.T) {
	if ModeDeclarative.String() != "declarative" {
		t.Error("ModeDeclarative.String()")
	}
	if ModeLegacy.String() != "legacy (session_dirs)" {
		t.Error("ModeLegacy.String()")
	}
	if ModeUnknown.String() != "unknown" {
		t.Error("ModeUnknown.String()")
	}
}

func TestCollapseWhitespace(t *testing.T) {
	cfg := TextConfig{Join: " ", Trim: true}
	got, err := ApplyTransforms(cfg, "hello\nworld\n  foo")
	if err != nil {
		t.Fatal(err)
	}
	// After join=" ", newlines become spaces
	if got == "" {
		t.Error("expected non-empty result")
	}
}

func TestToList_stringSlice(t *testing.T) {
	list, ok := toList([]string{"a", "b"})
	if !ok {
		t.Error("expected ok for []string")
	}
	if len(list) != 2 {
		t.Errorf("len = %d, want 2", len(list))
	}
}

func TestInvalidPatternError(t *testing.T) {
	cfg := TextConfig{
		Remove: []RemoveConfig{{Kind: "regex", Pattern: `[bad`}},
	}
	_, err := ApplyTransforms(cfg, "text")
	if err == nil {
		t.Fatal("expected error")
	}
	var pe *InvalidPatternError
	if !errors.As(err, &pe) {
		t.Fatalf("expected InvalidPatternError, got %T", err)
	}
	if pe.Error() == "" {
		t.Error("Error() should not be empty")
	}
	if pe.Unwrap() == nil {
		t.Error("Unwrap() should not be nil")
	}
}

func TestParseDeclarative_basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{
			"type":      "user",
			"uuid":      "u1",
			"timestamp": "2024-01-01T00:00:00Z",
			"sessionId": "s1",
			"message": map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "hello"}},
			},
		},
		{
			"type":      "assistant",
			"uuid":      "u2",
			"timestamp": "2024-01-01T00:01:00Z",
			"sessionId": "s1",
			"message": map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "text", "text": "world"}},
			},
		},
	}
	writeJSONL(t, path, records)

	msgs, err := ParseDeclarative(path, claudeDef())
	if err != nil {
		t.Fatalf("ParseDeclarative: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Errorf("unexpected roles: %v %v", msgs[0].Role, msgs[1].Role)
	}
	if msgs[0].Content != "hello" || msgs[1].Content != "world" {
		t.Errorf("unexpected content: %q %q", msgs[0].Content, msgs[1].Content)
	}
	if msgs[0].Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}
}

func TestParseDeclarative_filterExcluded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{
			"type":   "user",
			"isMeta": true, // should be excluded
			"message": map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "meta msg"}},
			},
		},
		{
			"type": "user",
			"message": map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "real msg"}},
			},
		},
	}
	writeJSONL(t, path, records)

	msgs, err := ParseDeclarative(path, claudeDef())
	if err != nil {
		t.Fatalf("ParseDeclarative: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (meta excluded), got %d", len(msgs))
	}
	if msgs[0].Content != "real msg" {
		t.Errorf("content = %q", msgs[0].Content)
	}
}

func TestParseDeclarative_roleNormalization(t *testing.T) {
	if normalizeRole("human") != "user" {
		t.Error("human should normalize to user")
	}
	if normalizeRole("user") != "user" {
		t.Error("user should remain user")
	}
	if normalizeRole("assistant") != "assistant" {
		t.Error("assistant should remain assistant")
	}
}

func TestParseDeclarative_invalidTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{
			"type":      "user",
			"timestamp": "not-a-timestamp",
			"message": map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "text", "text": "content"}},
			},
		},
	}
	writeJSONL(t, path, records)

	msgs, err := ParseDeclarative(path, claudeDef())
	if err != nil {
		t.Fatalf("ParseDeclarative: %v", err)
	}
	// Should succeed with time.Now() fallback
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Timestamp.IsZero() {
		t.Error("timestamp should not be zero even with bad input")
	}
}

func TestParseDeclarative_missingFile(t *testing.T) {
	_, err := ParseDeclarative("/nonexistent/path.jsonl", claudeDef())
	if err == nil {
		t.Error("expected error for missing file")
	}
}
