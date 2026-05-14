package input_config

import (
	"encoding/json"
	"testing"
)

// claudeRecord is a representative Claude JSONL record.
const claudeRecordJSON = `{
	"uuid": "msg-abc",
	"timestamp": "2024-01-02T03:04:05Z",
	"sessionId": "sess-1",
	"type": "assistant",
	"isMeta": false,
	"message": {
		"role": "assistant",
		"content": [
			{"type": "text", "text": "Hello world"},
			{"type": "tool_use", "id": "t1", "name": "Bash"}
		]
	}
}`

// piRecord is a representative Pi JSONL record.
const piRecordJSON = `{
	"id": "pi-uuid-1",
	"timestamp": "2024-02-01T10:00:00Z",
	"cwd": "/home/user/project",
	"type": "message",
	"message": {
		"role": "user",
		"content": [{"type": "text", "text": "What is Pi?"}]
	}
}`

func parseRecord(t *testing.T, raw string) map[string]any {
	t.Helper()
	var r map[string]any
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatalf("parse record: %v", err)
	}
	return r
}

func TestSelectField_TopLevel(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	v, ok := SelectField(rec, "$.uuid")
	if !ok || v != "msg-abc" {
		t.Errorf("$.uuid: ok=%v v=%v", ok, v)
	}

	v, ok = SelectField(rec, "$.type")
	if !ok || v != "assistant" {
		t.Errorf("$.type: ok=%v v=%v", ok, v)
	}
}

func TestSelectField_Nested(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	v, ok := SelectField(rec, "$.message.role")
	if !ok || v != "assistant" {
		t.Errorf("$.message.role: ok=%v v=%v", ok, v)
	}
}

func TestSelectField_Root(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	v, ok := SelectField(rec, "$")
	if !ok {
		t.Error("$ should return the whole record")
	}
	m, ok := v.(map[string]any)
	if !ok || m["uuid"] != "msg-abc" {
		t.Errorf("$ returned wrong value: %v", v)
	}
}

func TestSelectField_Empty(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	v, ok := SelectField(rec, "")
	if !ok {
		t.Error("empty path should return the whole record")
	}
	if v == nil {
		t.Error("empty path returned nil")
	}
}

func TestSelectField_ArrayIndex(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	v, ok := SelectField(rec, "$.message.content[0]")
	if !ok {
		t.Fatal("$.message.content[0] not found")
	}
	block, ok := v.(map[string]any)
	if !ok || block["type"] != "text" {
		t.Errorf("content[0] wrong: %v", v)
	}
}

func TestSelectField_ArrayWildcard(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	v, ok := SelectField(rec, "$.message.content[*]")
	if !ok {
		t.Fatal("$.message.content[*] not found")
	}
	elems, ok := v.([]any)
	if !ok {
		t.Fatalf("$.message.content[*] expected []any, got %T", v)
	}
	if len(elems) != 2 {
		t.Errorf("$.message.content[*] len = %d, want 2", len(elems))
	}
}

func TestSelectField_WildcardFieldMap(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	// $.message.content[*].type → ["text", "tool_use"]
	v, ok := SelectField(rec, "$.message.content[*].type")
	if !ok {
		t.Fatal("$.message.content[*].type not found")
	}
	types, ok := v.([]any)
	if !ok || len(types) != 2 {
		t.Errorf("wildcard map: %v", v)
	}
	if types[0] != "text" || types[1] != "tool_use" {
		t.Errorf("unexpected types: %v", types)
	}
}

func TestSelectField_Missing(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	_, ok := SelectField(rec, "$.nonexistent")
	if ok {
		t.Error("nonexistent path should return false")
	}

	_, ok = SelectField(rec, "$.message.nonexistent")
	if ok {
		t.Error("nested nonexistent path should return false")
	}
}

func TestSelectField_OutOfBounds(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	_, ok := SelectField(rec, "$.message.content[99]")
	if ok {
		t.Error("out-of-bounds index should return false")
	}
}

func TestSelectField_Pi(t *testing.T) {
	rec := parseRecord(t, piRecordJSON)

	v, ok := SelectField(rec, "$.id")
	if !ok || v != "pi-uuid-1" {
		t.Errorf("Pi $.id: ok=%v v=%v", ok, v)
	}

	v, ok = SelectField(rec, "$.cwd")
	if !ok || v != "/home/user/project" {
		t.Errorf("Pi $.cwd: ok=%v v=%v", ok, v)
	}
}

func TestSelectString_Bool(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	// isMeta is a bool false — should convert
	s, ok := SelectString(rec, "$.isMeta")
	if !ok || s != "false" {
		t.Errorf("bool SelectString: ok=%v s=%q", ok, s)
	}
}

func TestSelectString_Missing(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	s, ok := SelectString(rec, "$.doesNotExist")
	if ok || s != "" {
		t.Errorf("missing SelectString: ok=%v s=%q", ok, s)
	}
}

func TestSelectString_Nested(t *testing.T) {
	rec := parseRecord(t, claudeRecordJSON)

	s, ok := SelectString(rec, "$.message.role")
	if !ok || s != "assistant" {
		t.Errorf("$.message.role: ok=%v s=%q", ok, s)
	}
}
