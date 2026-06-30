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

const piFixture = "../../tests/fixtures/pi-session.jsonl"

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

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
	}

	r := &ClaudeReader{}

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
	// ClaudeReader may split text and tool blocks into separate messages
	// Just verify the fixture's text content is present
	var foundContent bool
	for _, rec := range pf.Records {
		if containsStr(rec.Content, "pipeline test") || containsStr(rec.Content, "pipeline response") {
			foundContent = true
		}
	}
	if !foundContent {
		t.Errorf("Parse: expected to find fixture content in records, got %d records", len(pf.Records))
	}
	if pf.Hash != hash {
		t.Error("ParsedFile.Hash != Hash()")
	}
}

// TestPipeline_PiJSONL tests the full pipeline with Pi JSONL format.
func TestPipeline_PiJSONL(t *testing.T) {
	r := &PiReader{}
	def := input_config.InputDefinition{}

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

// createTestOpenCodeDB creates a minimal OpenCode-format DB using the real anomalyco/opencode schema.
func createTestOpenCodeDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("create opencode db: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Exec(`
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
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}

	now := time.Now().UnixMilli()
	msgData, _ := json.Marshal(map[string]string{"role": "user"})
	_, err = db.Exec(
		`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`,
		"m1", "s1", now+1000, now+1000, string(msgData),
	)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}

	partData, _ := json.Marshal(map[string]string{"type": "text", "text": "integration test content"})
	_, err = db.Exec(
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?)`,
		"p1", "m1", "s1", now+1000, now+1000, string(partData),
	)
	if err != nil {
		t.Fatalf("insert part: %v", err)
	}
}
