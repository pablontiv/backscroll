package readers

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	_ "modernc.org/sqlite"
)

// createOpenCodeDB creates a temporary OpenCode-format SQLite database for testing.
func createOpenCodeDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			message_count INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0.0,
			updated_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			parts TEXT NOT NULL DEFAULT '[]',
			model TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			finished_at INTEGER,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	now := time.Now().UnixMilli()
	_, err = db.Exec(`INSERT INTO sessions (id, title, updated_at, created_at) VALUES (?, ?, ?, ?)`,
		"sess-1", "Test Session", now, now)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	type part struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}

	insertMsg := func(id, role string, parts []part, createdAt int64) {
		t.Helper()
		partsJSON, _ := json.Marshal(parts)
		_, err := db.Exec(
			`INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			id, "sess-1", role, string(partsJSON), createdAt, createdAt,
		)
		if err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}

	// User message with text part
	insertMsg("msg-1", "user", []part{
		{Type: "text", Data: map[string]string{"text": "hello from user"}},
		{Type: "finish", Data: map[string]string{"reason": "stop"}},
	}, now+1000)

	// Assistant message with text + tool_call parts (only text indexed)
	insertMsg("msg-2", "assistant", []part{
		{Type: "text", Data: map[string]string{"text": "I will use a tool"}},
		{Type: "tool_call", Data: map[string]string{"id": "tc1", "name": "bash"}},
	}, now+2000)

	// Tool result message (should produce empty content → skipped)
	insertMsg("msg-3", "tool", []part{
		{Type: "tool_result", Data: map[string]string{"id": "tc1", "output": "result"}},
		{Type: "finish", Data: map[string]string{"reason": "stop"}},
	}, now+3000)

	// Reasoning-only message (should be skipped)
	insertMsg("msg-4", "assistant", []part{
		{Type: "reasoning", Data: map[string]string{"thinking": "hidden thought"}},
	}, now+4000)

	return dbPath
}

func TestOpenCodeReader_Name(t *testing.T) {
	r := &OpenCodeReader{}
	if r.Name() != "opencode" {
		t.Errorf("Name() = %q, want opencode", r.Name())
	}
}

func TestOpenCodeReader_ImplementsSessionReader(t *testing.T) {
	var _ SessionReader = &OpenCodeReader{}
}

func TestOpenCodeReader_Hash(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	h, err := r.Hash(dbPath)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if h == "" || h == "empty" {
		t.Errorf("Hash = %q, want non-empty hex string", h)
	}
	// Should be 16 hex chars (int64 formatted as %016x)
	if len(h) != 16 {
		t.Errorf("Hash len = %d, want 16 (%%016x format): %q", len(h), h)
	}
}

func TestOpenCodeReader_Hash_MissingFile(t *testing.T) {
	r := &OpenCodeReader{}
	_, err := r.Hash("/nonexistent/opencode.db")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestOpenCodeReader_Parse_MessageCount(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// msg-1 (user, text) + msg-2 (assistant, text) = 2 messages
	// msg-3 (tool, only tool_result → empty) and msg-4 (reasoning only) are skipped
	if len(pf.Records) != 2 {
		t.Errorf("Records count = %d, want 2", len(pf.Records))
	}
}

func TestOpenCodeReader_Parse_Roles(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(pf.Records) < 2 {
		t.Fatalf("not enough records: %d", len(pf.Records))
	}
	if pf.Records[0].Role != "user" {
		t.Errorf("Record[0].Role = %q, want user", pf.Records[0].Role)
	}
	if pf.Records[1].Role != "assistant" {
		t.Errorf("Record[1].Role = %q, want assistant", pf.Records[1].Role)
	}
}

func TestOpenCodeReader_Parse_Content(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(pf.Records) < 1 {
		t.Fatalf("no records")
	}
	if pf.Records[0].Content != "hello from user" {
		t.Errorf("Record[0].Content = %q", pf.Records[0].Content)
	}
	if pf.Records[1].Content != "I will use a tool" {
		t.Errorf("Record[1].Content = %q", pf.Records[1].Content)
	}
}

func TestOpenCodeReader_Parse_ToolCallNotIndexed(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for _, rec := range pf.Records {
		if containsStr(rec.Content, "result") && containsStr(rec.Content, "tc1") {
			t.Error("tool_result content should not be indexed")
		}
		if containsStr(rec.Content, "hidden thought") {
			t.Error("reasoning content should not be indexed")
		}
	}
}

func TestOpenCodeReader_Parse_HashConsistency(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	h, err := r.Hash(dbPath)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if pf.Hash != h {
		t.Errorf("ParsedFile.Hash %q != Hash() %q", pf.Hash, h)
	}
}

func TestOpenCodeReader_Parse_Path(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Path != dbPath {
		t.Errorf("Path = %q, want %q", pf.Path, dbPath)
	}
}

func TestOpenCodeReader_Parse_Timestamps(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	zero := time.Time{}
	for i, rec := range pf.Records {
		if rec.Timestamp == zero {
			t.Errorf("Record[%d].Timestamp is zero", i)
		}
	}
}

func TestOpenCodeReader_Parse_MissingFile(t *testing.T) {
	r := &OpenCodeReader{}
	_, err := r.Parse("/nonexistent/opencode.db", input_config.InputDefinition{})
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestOpenCodeReader_Discover(t *testing.T) {
	dir := t.TempDir()
	// Create fake .opencode/opencode.db structure
	dbDir := filepath.Join(dir, ".opencode")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dbDir, "opencode.db"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &OpenCodeReader{}
	def := input_config.InputDefinition{
		Discover: input_config.DiscoverConfig{
			Roots:   []string{dir},
			Include: []string{"**/.opencode/opencode.db"},
		},
	}

	paths, err := r.Discover(def)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("Discover returned %d paths, want 1", len(paths))
	}
}

func TestExtractTextFromParts(t *testing.T) {
	type part struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}

	cases := []struct {
		name     string
		parts    []part
		wantText string
	}{
		{
			name: "text only",
			parts: []part{
				{Type: "text", Data: map[string]string{"text": "hello"}},
			},
			wantText: "hello",
		},
		{
			name: "text + tool_call",
			parts: []part{
				{Type: "text", Data: map[string]string{"text": "I will call"}},
				{Type: "tool_call", Data: map[string]string{"name": "bash"}},
			},
			wantText: "I will call",
		},
		{
			name:     "only tool_result",
			parts:    []part{{Type: "tool_result", Data: map[string]string{"output": "ok"}}},
			wantText: "",
		},
		{
			name:     "empty array",
			parts:    []part{},
			wantText: "",
		},
		{
			name: "multiple text parts",
			parts: []part{
				{Type: "text", Data: map[string]string{"text": "line1"}},
				{Type: "text", Data: map[string]string{"text": "line2"}},
			},
			wantText: "line1\nline2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(tc.parts)
			got := extractTextFromParts(string(raw))
			if got != tc.wantText {
				t.Errorf("got %q, want %q", got, tc.wantText)
			}
		})
	}
}

func TestExtractTextFromParts_Malformed(t *testing.T) {
	got := extractTextFromParts("not json")
	if got != "" {
		t.Errorf("malformed JSON should return empty, got %q", got)
	}
}
