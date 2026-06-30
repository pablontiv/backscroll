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
// Uses the real anomalyco/opencode schema: message + part tables with JSON data columns.
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

	insertMsg := func(id, role string, timeCreated int64) {
		t.Helper()
		data, _ := json.Marshal(map[string]string{"role": role})
		_, err := db.Exec(
			`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`,
			id, "sess-1", timeCreated, timeCreated, string(data),
		)
		if err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}

	insertPart := func(id, msgID string, data any) {
		t.Helper()
		dataJSON, _ := json.Marshal(data)
		ts := time.Now().UnixMilli()
		_, err := db.Exec(
			`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?, ?)`,
			id, msgID, "sess-1", ts, ts, string(dataJSON),
		)
		if err != nil {
			t.Fatalf("insert part %s: %v", id, err)
		}
	}

	// msg-1: user with text + step-finish (only text indexed)
	insertMsg("msg-1", "user", now+1000)
	insertPart("p-1-1", "msg-1", map[string]string{"type": "text", "text": "hello from user"})
	insertPart("p-1-2", "msg-1", map[string]string{"type": "step-finish"})

	// msg-2: assistant with text + tool (only text indexed)
	insertMsg("msg-2", "assistant", now+2000)
	insertPart("p-2-1", "msg-2", map[string]string{"type": "text", "text": "I will use a tool"})
	insertPart("p-2-2", "msg-2", map[string]interface{}{"type": "tool-use", "id": "tc1", "name": "bash"})

	// msg-3: tool result only (no text parts → skipped)
	insertMsg("msg-3", "tool", now+3000)
	insertPart("p-3-1", "msg-3", map[string]interface{}{"type": "tool-result", "id": "tc1", "output": "result"})

	// msg-4: assistant reasoning only (skipped)
	insertMsg("msg-4", "assistant", now+4000)
	insertPart("p-4-1", "msg-4", map[string]string{"type": "reasoning", "thinking": "hidden thought"})

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
	if len(h) != 16 {
		t.Errorf("Hash len = %d, want 16 (%%016x format): %q", len(h), h)
	}
}

func TestOpenCodeReader_Hash_Empty(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "empty.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL)`)
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	r := &OpenCodeReader{}
	h, err := r.Hash(dbPath)
	if err != nil {
		t.Fatalf("Hash empty: %v", err)
	}
	if h != "empty" {
		t.Errorf("Hash empty DB = %q, want %q", h, "empty")
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
	// msg-3 (only tool-result) and msg-4 (only reasoning) are skipped
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

	if len(pf.Records) < 2 {
		t.Fatalf("not enough records: %d", len(pf.Records))
	}
	if pf.Records[0].Content != "hello from user" {
		t.Errorf("Record[0].Content = %q", pf.Records[0].Content)
	}
	if pf.Records[1].Content != "I will use a tool" {
		t.Errorf("Record[1].Content = %q", pf.Records[1].Content)
	}
}

func TestOpenCodeReader_Parse_NonTextNotIndexed(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	r := &OpenCodeReader{}

	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for _, rec := range pf.Records {
		if containsStr(rec.Content, "result") && containsStr(rec.Content, "tc1") {
			t.Error("tool-result content should not be indexed")
		}
		if containsStr(rec.Content, "hidden thought") {
			t.Error("reasoning content should not be indexed")
		}
	}
}

func TestOpenCodeReader_Parse_Ignored(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
		CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
	`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	msgData, _ := json.Marshal(map[string]string{"role": "assistant"})
	_, _ = db.Exec(`INSERT INTO message VALUES (?, ?, ?, ?, ?)`, "m1", "s1", now, now, string(msgData))

	trueVal := true
	ignoredData, _ := json.Marshal(map[string]interface{}{"type": "text", "text": "hidden ignored text", "ignored": trueVal})
	visibleData, _ := json.Marshal(map[string]interface{}{"type": "text", "text": "visible text"})
	_, _ = db.Exec(`INSERT INTO part VALUES (?, ?, ?, ?, ?, ?)`, "p1", "m1", "s1", now, now, string(ignoredData))
	_, _ = db.Exec(`INSERT INTO part VALUES (?, ?, ?, ?, ?, ?)`, "p2", "m1", "s1", now+1, now+1, string(visibleData))
	_ = db.Close()

	r := &OpenCodeReader{}
	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pf.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(pf.Records))
	}
	if containsStr(pf.Records[0].Content, "hidden ignored text") {
		t.Error("ignored part should not appear in content")
	}
	if pf.Records[0].Content != "visible text" {
		t.Errorf("content = %q, want %q", pf.Records[0].Content, "visible text")
	}
}

func TestOpenCodeReader_Parse_MultipleTextParts(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
		CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
	`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	msgData, _ := json.Marshal(map[string]string{"role": "assistant"})
	_, _ = db.Exec(`INSERT INTO message VALUES (?, ?, ?, ?, ?)`, "m1", "s1", now, now, string(msgData))

	p1, _ := json.Marshal(map[string]string{"type": "text", "text": "line1"})
	p2, _ := json.Marshal(map[string]string{"type": "text", "text": "line2"})
	_, _ = db.Exec(`INSERT INTO part VALUES (?, ?, ?, ?, ?, ?)`, "p1", "m1", "s1", now, now, string(p1))
	_, _ = db.Exec(`INSERT INTO part VALUES (?, ?, ?, ?, ?, ?)`, "p2", "m1", "s1", now+1, now+1, string(p2))
	_ = db.Close()

	r := &OpenCodeReader{}
	pf, err := r.Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(pf.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(pf.Records))
	}
	if pf.Records[0].Content != "line1\nline2" {
		t.Errorf("Content = %q, want %q", pf.Records[0].Content, "line1\nline2")
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
		if rec.Timestamp.Equal(zero) {
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

// createOpenCodeDBWithTool builds a DB with one assistant message that has a
// text part and a real `type:"tool"` part (state.input object + state.output
// string), matching the live OpenCode schema.
func createOpenCodeDBWithTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
		CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
	`); err != nil {
		t.Fatalf("schema: %v", err)
	}

	now := time.Now().UnixMilli()
	msgData, _ := json.Marshal(map[string]string{"role": "assistant"})
	if _, err := db.Exec(`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?,?,?,?,?)`,
		"m1", "s1", now, now, string(msgData)); err != nil {
		t.Fatalf("insert msg: %v", err)
	}

	insertPart := func(id string, data any) {
		t.Helper()
		dj, _ := json.Marshal(data)
		if _, err := db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?,?,?,?,?,?)`,
			id, "m1", "s1", now, now, string(dj)); err != nil {
			t.Fatalf("insert part %s: %v", id, err)
		}
	}
	insertPart("p1", map[string]any{"type": "text", "text": "running a command"})
	insertPart("p2", map[string]any{
		"type": "tool",
		"tool": "bash",
		"state": map[string]any{
			"status": "completed",
			"input":  map[string]any{"command": "occ_marker_cmd", "description": "do it"},
			"output": "occ_output_token done",
		},
	})
	return dbPath
}

func TestOpenCodeReader_CapturesToolInputOutput(t *testing.T) {
	pf, err := (&OpenCodeReader{}).Parse(createOpenCodeDBWithTool(t), input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var gotText, gotInput, gotOutput bool
	for _, m := range pf.Records {
		switch {
		case m.ContentType == "text" && m.Content == "running a command":
			gotText = true
		case m.ContentType == "tool" && contains(m.Content, "occ_marker_cmd") && contains(m.Content, "bash"):
			gotInput = true
		case m.ContentType == "tool" && contains(m.Content, "occ_output_token"):
			gotOutput = true
		}
	}
	if !gotText {
		t.Error("missing text message")
	}
	if !gotInput {
		t.Error("missing tool input message")
	}
	if !gotOutput {
		t.Error("missing tool output message")
	}
}

func TestOpenCodeReader_ToolOnlyMessageEmitted(t *testing.T) {
	// A message with only a tool part (no text) must still produce tool messages.
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "opencode.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`
		CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
		CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
	`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	md, _ := json.Marshal(map[string]string{"role": "assistant"})
	if _, err := db.Exec(`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?,?,?,?,?)`, "m1", "s1", now, now, string(md)); err != nil {
		t.Fatal(err)
	}
	pdj, _ := json.Marshal(map[string]any{"type": "tool", "tool": "read", "state": map[string]any{"input": map[string]any{"file_path": "occ_only_marker"}, "output": ""}})
	if _, err := db.Exec(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES (?,?,?,?,?,?)`, "p1", "m1", "s1", now, now, string(pdj)); err != nil {
		t.Fatal(err)
	}

	pf, err := (&OpenCodeReader{}).Parse(dbPath, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	var gotInput bool
	for _, m := range pf.Records {
		if m.ContentType == "tool" && contains(m.Content, "occ_only_marker") {
			gotInput = true
		}
	}
	if !gotInput {
		t.Errorf("tool-only message not emitted; records = %+v", pf.Records)
	}
}
