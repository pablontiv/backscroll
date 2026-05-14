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

// TestPipeline_ClaudeJSONL tests the full Discover→Hash→Parse pipeline for Claude JSONL.
func TestPipeline_ClaudeJSONL(t *testing.T) {
	dir := t.TempDir()

	// Write a Claude-format JSONL session
	records := []map[string]any{
		{
			"type":      "user",
			"uuid":      "u1",
			"timestamp": "2024-06-01T10:00:00Z",
			"sessionId": "sess-a",
			"message":   map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "pipeline test"}}},
		},
		{
			"type":      "assistant",
			"uuid":      "u2",
			"timestamp": "2024-06-01T10:00:01Z",
			"sessionId": "sess-a",
			"message":   map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "pipeline response"}}},
		},
	}
	writeSession(t, filepath.Join(dir, "session.jsonl"), records)

	def := input_config.InputDefinition{
		Source: "session",
		Discover: input_config.DiscoverConfig{
			Roots:   []string{dir},
			Include: []string{"**/*.jsonl"},
		},
		Decode: input_config.DecodeConfig{Format: "jsonl"},
		Record: input_config.RecordConfig{
			IncludeWhen: []input_config.Predicate{
				{Selector: "$.type", Op: "in", Value: []any{"user", "assistant"}},
			},
		},
		Map: input_config.MapConfig{
			Role:      "$.message.role",
			UUID:      "$.uuid",
			Timestamp: "$.timestamp",
			SessionID: "$.sessionId",
		},
		Content: input_config.ContentConfig{
			Selector:  "$.message.content",
			BlockText: "$.text",
			IncludeWhen: []input_config.Predicate{
				{Selector: "$.type", Op: "eq", Value: "text"},
			},
		},
		Text: input_config.TextConfig{Trim: true, DropEmpty: true},
	}

	r := &JsonlReader{}

	refs, err := r.Discover(def)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("Discover: got %d refs, want 1", len(refs))
	}

	hash, err := r.Hash(refs[0])
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "" {
		t.Error("Hash should not be empty")
	}

	pf, err := r.Parse(refs[0], def)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pf.Records) != 2 {
		t.Errorf("Parse: got %d records, want 2", len(pf.Records))
	}
	if pf.Records[0].Content != "pipeline test" {
		t.Errorf("Record[0].Content = %q", pf.Records[0].Content)
	}
	if pf.Hash != hash {
		t.Error("ParsedFile.Hash != Hash()")
	}
}

// TestPipeline_PiJSONL tests the full pipeline with Pi JSONL format.
func TestPipeline_PiJSONL(t *testing.T) {
	r := &JsonlReader{}
	def := piDef()

	refs, err := r.Discover(input_config.InputDefinition{
		Discover: input_config.DiscoverConfig{
			Roots:   []string{filepath.Dir(piFixture)},
			Include: []string{"pi-session.jsonl"},
		},
	})
	if err != nil {
		t.Fatalf("Discover Pi: %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("Discover Pi: no refs found")
	}

	pf, err := r.Parse(refs[0], def)
	if err != nil {
		t.Fatalf("Parse Pi: %v", err)
	}
	if len(pf.Records) == 0 {
		t.Error("Pi parse: expected records")
	}
	// Verify thinking block excluded
	for _, rec := range pf.Records {
		if containsStr(rec.Content, "hidden reasoning") {
			t.Error("Pi: thinking block leaked")
		}
	}
}

// TestPipeline_OpenCode tests the full pipeline for OpenCode SQLite.
func TestPipeline_OpenCode(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "opencode.db")
	createTestOpenCodeDB(t, dbPath)

	def := input_config.InputDefinition{
		Source: "session",
		Discover: input_config.DiscoverConfig{
			Roots:   []string{dbDir},
			Include: []string{"*.db"},
		},
		Decode: input_config.DecodeConfig{Format: "opencode"},
	}

	r := &OpenCodeReader{}

	refs, err := r.Discover(def)
	if err != nil {
		t.Fatalf("Discover OpenCode: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("Discover OpenCode: got %d refs, want 1", len(refs))
	}

	hash1, err := r.Hash(refs[0])
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	hash2, err := r.Hash(refs[0])
	if err != nil {
		t.Fatalf("Hash 2: %v", err)
	}
	if hash1 != hash2 {
		t.Error("Hash not deterministic")
	}

	pf, err := r.Parse(refs[0], def)
	if err != nil {
		t.Fatalf("Parse OpenCode: %v", err)
	}
	if len(pf.Records) == 0 {
		t.Error("OpenCode: expected records")
	}
}

// TestPipeline_Dedup verifies that hashing is stable for unchanged content.
func TestPipeline_Dedup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	content := `{"type":"user","uuid":"x","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &JsonlReader{}

	h1, err := r.Hash(path)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := r.Hash(path)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Error("Hash not stable across calls")
	}

	// Modify the file
	if err := os.WriteFile(path, []byte(content+"extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h3, err := r.Hash(path)
	if err != nil {
		t.Fatal(err)
	}
	if h3 == h1 {
		t.Error("Hash should change when content changes")
	}
}

// TestPipeline_Predicates verifies that record predicates filter records.
func TestPipeline_Predicates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{"type": "user", "message": map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "include me"}}}},
		{"type": "system-reminder", "message": map[string]any{"role": "system", "content": []map[string]any{{"type": "text", "text": "exclude me"}}}},
		{"type": "assistant", "isMeta": true, "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "also excluded"}}}},
	}
	writeSession(t, path, records)

	def := input_config.InputDefinition{
		Discover: input_config.DiscoverConfig{Roots: []string{dir}, Include: []string{"*.jsonl"}},
		Decode:   input_config.DecodeConfig{Format: "jsonl"},
		Record: input_config.RecordConfig{
			IncludeWhen: []input_config.Predicate{
				{Selector: "$.type", Op: "in", Value: []any{"user", "assistant"}},
			},
			ExcludeWhen: []input_config.Predicate{
				{Selector: "$.isMeta", Op: "eq", Value: true},
			},
		},
		Map: input_config.MapConfig{
			Role:      "$.message.role",
			Timestamp: "$.timestamp",
		},
		Content: input_config.ContentConfig{
			Selector:  "$.message.content",
			BlockText: "$.text",
			IncludeWhen: []input_config.Predicate{
				{Selector: "$.type", Op: "eq", Value: "text"},
			},
		},
		Text: input_config.TextConfig{Trim: true, DropEmpty: true},
	}

	r := &JsonlReader{}
	pf, err := r.Parse(path, def)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(pf.Records) != 1 {
		t.Errorf("expected 1 record (only user type, not meta, not system), got %d", len(pf.Records))
	}
	if len(pf.Records) > 0 && pf.Records[0].Content != "include me" {
		t.Errorf("wrong record: %q", pf.Records[0].Content)
	}
}

// TestPipeline_TextTransforms verifies text transforms are applied end-to-end.
func TestPipeline_TextTransforms(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	records := []map[string]any{
		{
			"type":    "user",
			"message": map[string]any{"role": "user", "content": []map[string]any{{"type": "text", "text": "  hello SECRET world  "}}},
		},
	}
	writeSession(t, path, records)

	def := input_config.InputDefinition{
		Discover: input_config.DiscoverConfig{Roots: []string{dir}, Include: []string{"*.jsonl"}},
		Decode:   input_config.DecodeConfig{Format: "jsonl"},
		Record: input_config.RecordConfig{
			IncludeWhen: []input_config.Predicate{
				{Selector: "$.type", Op: "in", Value: []any{"user", "assistant"}},
			},
		},
		Map:     input_config.MapConfig{Role: "$.message.role"},
		Content: input_config.ContentConfig{Selector: "$.message.content", BlockText: "$.text", IncludeWhen: []input_config.Predicate{{Selector: "$.type", Op: "eq", Value: "text"}}},
		Text: input_config.TextConfig{
			Trim:      true,
			DropEmpty: true,
			Remove:    []input_config.RemoveConfig{{Kind: "substring", Pattern: "SECRET "}},
		},
	}

	r := &JsonlReader{}
	pf, err := r.Parse(path, def)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(pf.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(pf.Records))
	}
	if containsStr(pf.Records[0].Content, "SECRET") {
		t.Errorf("transform not applied: %q", pf.Records[0].Content)
	}
	if !containsStr(pf.Records[0].Content, "hello") || !containsStr(pf.Records[0].Content, "world") {
		t.Errorf("content mangled: %q", pf.Records[0].Content)
	}
}

// writeSession writes JSONL records to a file.
func writeSession(t *testing.T, path string, records []map[string]any) {
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

// createTestOpenCodeDB creates a minimal OpenCode-format DB.
func createTestOpenCodeDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("create opencode db: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY, title TEXT NOT NULL, message_count INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0.0, updated_at INTEGER NOT NULL, created_at INTEGER NOT NULL);
		CREATE TABLE messages (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, role TEXT NOT NULL,
			parts TEXT NOT NULL DEFAULT '[]', model TEXT, created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL, finished_at INTEGER);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	now := time.Now().UnixMilli()
	_, err = db.Exec(`INSERT INTO sessions (id, title, updated_at, created_at) VALUES (?, ?, ?, ?)`,
		"s1", "Integration Session", now, now)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}

	type part struct {
		Type string `json:"type"`
		Data any    `json:"data"`
	}
	parts, _ := json.Marshal([]part{{Type: "text", Data: map[string]string{"text": "integration test content"}}})
	_, err = db.Exec(`INSERT INTO messages (id, session_id, role, parts, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"m1", "s1", "user", string(parts), now+1000, now+1000)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
}
