package readers

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	_ "modernc.org/sqlite"
)

func TestOpenCodeReader_MessageOrigins(t *testing.T) {
	dbPath := createOpenCodeOriginDB(t)

	parsed, err := (&OpenCodeReader{}).Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	want := map[string]struct {
		role   string
		origin models.MessageOrigin
	}{
		"assistant automation system": {role: "user", origin: models.OriginHuman},
		"user human":                  {role: "assistant", origin: models.OriginAssistant},
		"system text":                 {role: "system", origin: models.OriginUnknown},
		"tool role text":              {role: "tool", origin: models.OriginUnknown},
		"developer text":              {role: "developer", origin: models.OriginUnknown},
	}

	for _, record := range parsed.Records {
		expected, ok := want[record.Content]
		if !ok {
			continue
		}
		if record.Role != expected.role || record.Origin != expected.origin {
			t.Errorf("record %q: role/origin = %q/%q, want %q/%q", record.Content, record.Role, record.Origin, expected.role, expected.origin)
		}
		delete(want, record.Content)
	}
	for content := range want {
		t.Errorf("missing text record %q", content)
	}
}

func TestOpenCodeReader_ToolOriginsUseStructuredBoundaries(t *testing.T) {
	dbPath := createOpenCodeOriginDB(t)

	parsed, err := (&OpenCodeReader{}).Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var input, output *models.Message
	for i := range parsed.Records {
		record := &parsed.Records[i]
		if record.ContentType != "tool" {
			continue
		}
		switch {
		case containsStr(record.Content, "opencode_origin_input"):
			input = record
		case containsStr(record.Content, "opencode_origin_output"):
			output = record
		}
	}

	if input == nil {
		t.Fatal("missing tool input record")
	}
	if input.Role != "assistant" || input.Origin != models.OriginAssistant {
		t.Errorf("tool input role/origin = %q/%q, want assistant/assistant", input.Role, input.Origin)
	}
	if output == nil {
		t.Fatal("missing explicit tool output record")
	}
	if output.Role != "assistant" || output.Origin != models.OriginAutomation {
		t.Errorf("tool output role/origin = %q/%q, want assistant/automation", output.Role, output.Origin)
	}
}

func createOpenCodeOriginDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`
		CREATE TABLE message (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);
		CREATE TABLE part (
			id TEXT PRIMARY KEY,
			message_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			time_created INTEGER NOT NULL,
			time_updated INTEGER NOT NULL,
			data TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	now := time.Now().UnixMilli()
	insertMessage := func(id, role string, timestamp int64) {
		t.Helper()
		data, err := json.Marshal(map[string]string{"role": role})
		if err != nil {
			t.Fatalf("marshal message %s: %v", id, err)
		}
		if _, err := db.Exec(
			`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`,
			id, "session", timestamp, timestamp, string(data),
		); err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}
	insertPart := func(id, messageID string, timestamp int64, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal part %s: %v", id, err)
		}
		if _, err := db.Exec(
			`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?)`,
			id, messageID, "session", timestamp, timestamp, string(data),
		); err != nil {
			t.Fatalf("insert part %s: %v", id, err)
		}
	}

	insertMessage("m-user", "user", now)
	insertPart("p-user", "m-user", now, map[string]any{"type": "text", "text": "assistant automation system"})

	insertMessage("m-assistant", "assistant", now+1)
	insertPart("p-assistant-text", "m-assistant", now+1, map[string]any{"type": "text", "text": "user human"})
	insertPart("p-assistant-tool", "m-assistant", now+1, map[string]any{
		"type": "tool",
		"tool": "bash",
		"state": map[string]any{
			"input":  map[string]any{"command": "echo opencode_origin_input user human"},
			"output": "opencode_origin_output assistant human",
		},
	})

	insertMessage("m-system", "system", now+2)
	insertPart("p-system", "m-system", now+2, map[string]any{"type": "text", "text": "system text"})
	insertMessage("m-tool", "tool", now+3)
	insertPart("p-tool", "m-tool", now+3, map[string]any{"type": "text", "text": "tool role text"})
	insertMessage("m-developer", "developer", now+4)
	insertPart("p-developer", "m-developer", now+4, map[string]any{"type": "text", "text": "developer text"})

	return dbPath
}
