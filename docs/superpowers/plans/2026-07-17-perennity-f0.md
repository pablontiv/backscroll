# F0 Perennity Cycle (F0a + F0b) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **Delivery layer:** this plan executes under `slice-orchestration`. Writers NEVER commit, push, or stage — the orchestrator makes one clean commit per slice after review. Every "Commit" step below is therefore expressed as "Report STATUS to orchestrator".

**Goal:** Make the backscroll DB a perennial event store: capture uuid/tool metadata/interrupt evidence at reader level (F0a), and switch session sync to append-only upsert with a non-destructive rebuild (F0b), so indexed sessions and future annotations survive source-file expiry.

**Architecture:** Rich capture happens in readers before serialization/cleaning destroys evidence; new fields flow `models.Message` → `storage.IndexedMessage` → `search_items` + new perennial `tool_events` satellite table (migration v8). Session files whose messages all carry uuids sync via INSERT OR IGNORE (stable ids); everything else keeps today's wipe-and-reload. `rebuild` stops purging and instead re-derives FTS from the DB.

**Tech Stack:** Go stdlib, modernc.org/sqlite (no CGO), stdlib testing, existing repo helpers (`sync.IterateJSONLFile`, `hashfile`, `toolfmt`).

**Spec:** `docs/superpowers/specs/2026-07-17-pattern-discovery-northstar-design.md` (slices F0a, F0b).

## Global Constraints

- Pure Go, no CGO. Never add dependencies.
- Migration rule: every schema change is a NEW migration version (v8 here); never modify existing migration blocks (verbatim from CLAUDE.md).
- Tests hermetic: `t.Setenv("HOME", t.TempDir())` where machine state could leak; must pass under `just ci` (scrubbed HOME).
- Aggregate coverage ≥85% (`just ci` gate).
- `gofmt` + `go vet` clean (`just check`).
- Perennity invariant (from spec): rows in `search_items` are never deleted except by `purge`; `tool_events` is perennial (no CASCADE lifecycle); agent work must never be silently destroyed.
- Writers report STATUS only; no git operations by writers.

---

## Slice 1 — Migration v8 + model plumbing

### Task 1: Migration v8 (extraction_version, was_interrupted, tool_events)

**Files:**
- Modify: `internal/storage/migrations.go` (append new version-check block in `SetupSchema` after the v7 block at ~line 97-107; append `applyV8Migration` at end of file)
- Test: `internal/storage/migrations_v8_test.go` (create)

**Interfaces:**
- Produces: table `tool_events(id, message_uuid TEXT, source_path TEXT NOT NULL, ordinal INTEGER NOT NULL, tool_name TEXT NOT NULL, command_head TEXT, is_error INTEGER, exit_code INTEGER, extraction_version INTEGER NOT NULL, UNIQUE(source_path, ordinal))`; columns `search_items.extraction_version INTEGER`, `search_items.was_interrupted INTEGER`. Later tasks rely on these exact names.

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func TestV8MigrationAddsToolEventsAndColumns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// tool_events exists with expected columns
	if _, err := db.db.Exec(`INSERT INTO tool_events
		(message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
		VALUES ('u1', '/p/s.jsonl', 0, 'Bash', 'go', 1, NULL, 1)`); err != nil {
		t.Fatalf("insert tool_events: %v", err)
	}

	// UNIQUE(source_path, ordinal) enforced
	if _, err := db.db.Exec(`INSERT INTO tool_events
		(message_uuid, source_path, ordinal, tool_name, extraction_version)
		VALUES ('u2', '/p/s.jsonl', 0, 'Read', 1)`); err == nil {
		t.Fatal("expected UNIQUE(source_path, ordinal) violation")
	}

	// new search_items columns accept values
	if _, err := db.db.Exec(`INSERT INTO search_items
		(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
		VALUES ('session', '/p/s.jsonl', 0, 'user', 'hi', '2026-01-01T00:00:00Z', 'u9', 'proj', 'text', 1, 1)`); err != nil {
		t.Fatalf("insert search_items with v8 columns: %v", err)
	}

	// migration recorded
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 8").Scan(&n); err != nil || n != 1 {
		t.Fatalf("v8 not recorded: n=%d err=%v", n, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestV8MigrationAddsToolEventsAndColumns ./internal/storage/`
Expected: FAIL with "no such table: tool_events"

- [ ] **Step 3: Implement migration**

In `SetupSchema`, after the v7 block (copy the v7 block's shape exactly):

```go
	// Check if version 8 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 8").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 8: %w", err)
	}

	if count == 0 {
		if err := d.applyV8Migration(); err != nil {
			return err
		}
	}
```

At end of file:

```go
// applyV8Migration adds the F0 perennity surface: extraction_version and
// was_interrupted on search_items, and the perennial tool_events satellite
// table (one row per tool_use, anchored by message identity). tool_events is
// NOT re-derivable once source files expire — no CASCADE lifecycle; only
// purge deletes from it, explicitly.
func (d *Database) applyV8Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	const sqlV8 = `
ALTER TABLE search_items ADD COLUMN extraction_version INTEGER;
ALTER TABLE search_items ADD COLUMN was_interrupted INTEGER;
CREATE TABLE IF NOT EXISTS tool_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_uuid TEXT,
    source_path TEXT NOT NULL,
    ordinal INTEGER NOT NULL,
    tool_name TEXT NOT NULL,
    command_head TEXT,
    is_error INTEGER,
    exit_code INTEGER,
    extraction_version INTEGER NOT NULL,
    UNIQUE(source_path, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_tool_events_tool ON tool_events(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_events_uuid ON tool_events(message_uuid);
`
	if _, err := tx.Exec(sqlV8); err != nil {
		return fmt.Errorf("apply v8 perennity schema: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV8))
	checksumHex := fmt.Sprintf("%x", checksum)

	if _, err := tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (8, 'V8 perennity: extraction_version, was_interrupted, tool_events', CURRENT_TIMESTAMP, ?)
	`, checksumHex); err != nil {
		return fmt.Errorf("record migration v8: %w", err)
	}

	return tx.Commit()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestV8MigrationAddsToolEventsAndColumns ./internal/storage/`
Expected: PASS

- [ ] **Step 5: Run full storage suite + vet**

Run: `go test ./internal/storage/ && just check`
Expected: all PASS, no vet errors

- [ ] **Step 6: Report STATUS to orchestrator** (files touched, tests run, PASS/FAIL — no git operations)

### Task 2: Model fields (models.Message, storage.IndexedMessage)

**Files:**
- Modify: `internal/models/models.go:31-37` (Message struct)
- Modify: `internal/storage/sync.go:7-15` (IndexedMessage struct)

**Interfaces:**
- Produces (exact fields later tasks consume):
  - `models.Message`: adds `UUID string`, `ToolName string`, `CommandHead string`, `ToolUseID string`, `IsError *bool`, `WasInterrupted bool`
  - `storage.IndexedMessage`: adds `ToolName string`, `CommandHead string`, `IsError *bool`, `WasInterrupted bool`, `ExtractionVersion int` (UUID already exists)
  - `storage.CurrentExtractionVersion = 1` (exported const in `internal/storage/sync.go`)

- [ ] **Step 1: Extend models.Message**

```go
// Message represents a message in a session.
type Message struct {
	Role        string
	Content     string
	ContentType string
	Timestamp   time.Time
	// F0a rich capture (perennity: not recoverable once source files expire)
	UUID           string // per-message identity: record uuid, or uuid#tN / uuid#rN for tool blocks
	ToolName       string // tool_use messages only
	CommandHead    string // first token of a command-like tool input ("" otherwise)
	ToolUseID      string // tool_use block id; used to pair tool_result is_error signals
	IsError        *bool  // tool result signal; nil = no signal
	WasInterrupted bool   // raw content carried an interrupt marker (detected pre-clean)
}
```

- [ ] **Step 2: Extend storage.IndexedMessage and add version const**

```go
// CurrentExtractionVersion identifies the reader-extraction logic that
// produced a row. Bump when extraction semantics change; rows keep the
// version that actually produced them (perennity: old rows are never
// silently reinterpreted).
const CurrentExtractionVersion = 1

// IndexedMessage represents a message to be indexed.
type IndexedMessage struct {
	Ordinal     int
	Role        string
	Text        string
	UUID        string
	Timestamp   string
	ContentType string
	// F0a rich capture
	ToolName          string
	CommandHead       string
	IsError           *bool
	WasInterrupted    bool
	ExtractionVersion int
}
```

- [ ] **Step 3: Compile check**

Run: `go build ./... && just check`
Expected: clean build (fields unused yet is fine)

- [ ] **Step 4: Report STATUS to orchestrator**

---

## Slice 2 — Claude reader rich capture (F0a)

### Task 3: Extract uuid, tool metadata, interrupt flag in ClaudeReader

**Files:**
- Modify: `internal/readers/claude_reader.go` (structs at lines 28-48; `Parse` at 55-82; `extractClaudeMessages` at 84-133)
- Test: `internal/readers/claude_reader_capture_test.go` (create)

**Interfaces:**
- Consumes: Task 2's `models.Message` fields.
- Produces: `models.ParsedFile.Records` where messages carry `UUID` (record uuid; tool blocks get `uuid#t<i>` / `uuid#r<i>` suffix by block index), `ToolName`, `CommandHead`, `IsError` (paired from tool_result via tool_use_id, including cross-record pairing), `WasInterrupted`. Helper `commandHead(input json.RawMessage) string`.

- [ ] **Step 1: Write the failing test**

```go
package readers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
)

const captureFixture = `{"type":"user","uuid":"aaa-1","timestamp":"2026-01-01T10:00:00Z","cwd":"/w","message":{"role":"user","content":"Caveat: x Request interrupted no, te pedi otra cosa"}}
{"type":"assistant","uuid":"aaa-2","timestamp":"2026-01-01T10:00:05Z","message":{"role":"assistant","content":[{"type":"text","text":"running"},{"type":"tool_use","id":"tu-9","name":"Bash","input":{"command":"go test ./...","description":"run tests"}}]}}
{"type":"user","uuid":"aaa-3","timestamp":"2026-01-01T10:00:09Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-9","content":"FAIL","is_error":true}]}}
`

func TestClaudeReaderRichCapture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(captureFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	pf, err := (&ClaudeReader{}).Parse(path, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pf.Records) != 4 { // user text, assistant text, tool_use, tool_result
		t.Fatalf("want 4 records, got %d: %+v", len(pf.Records), pf.Records)
	}

	user := pf.Records[0]
	if user.UUID != "aaa-1" {
		t.Errorf("user uuid = %q, want aaa-1", user.UUID)
	}
	if !user.WasInterrupted {
		t.Error("interrupt marker must be captured before CleanContent strips it")
	}

	var toolUse, toolResult *models.Message
	for i := range pf.Records {
		m := &pf.Records[i]
		switch {
		case m.ToolName == "Bash":
			toolUse = m
		case m.IsError != nil && m.ToolName == "":
			toolResult = m
		}
	}
	if toolUse == nil {
		t.Fatal("no tool_use message captured")
	}
	if toolUse.UUID != "aaa-2#t1" {
		t.Errorf("tool_use uuid = %q, want aaa-2#t1", toolUse.UUID)
	}
	if toolUse.CommandHead != "go" {
		t.Errorf("command_head = %q, want go", toolUse.CommandHead)
	}
	if toolUse.IsError == nil || !*toolUse.IsError {
		t.Error("tool_use is_error must be paired from the later tool_result (cross-record)")
	}
	if toolResult == nil || toolResult.UUID != "aaa-3#r0" {
		t.Fatalf("tool_result missing or wrong uuid: %+v", toolResult)
	}
}
```

The test file's single import block must include `"github.com/pablontiv/backscroll/internal/models"` alongside os/path/filepath/testing/input_config.

Note the pairing subtlety the assertions encode: after cross-record pairing, the tool_use message ALSO has `IsError != nil`, so the tool_result is distinguished by `ToolName == ""`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestClaudeReaderRichCapture ./internal/readers/`
Expected: FAIL (uuid empty, WasInterrupted false, IsError nil)

- [ ] **Step 3: Implement capture**

Struct changes:

```go
type claudeRecord struct {
	Type      string         `json:"type"`
	UUID      string         `json:"uuid"`
	Timestamp string         `json:"timestamp"`
	CWD       string         `json:"cwd"`
	IsMeta    bool           `json:"isMeta"`
	Message   *claudeMessage `json:"message"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
	Content   json.RawMessage `json:"content"`
	IsError   *bool           `json:"is_error"`
}
```

New helpers (same file):

```go
// interruptMarker is detected on RAW content, before sync.CleanContent
// removes the evidence (perennity: this signal is otherwise unrecoverable).
const interruptMarker = "Request interrupted"

// commandHead extracts the first whitespace token of a command-like tool
// input ({"command": "go test ./..."} -> "go"). Empty for non-command inputs.
func commandHead(input json.RawMessage) string {
	var obj struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &obj); err != nil || obj.Command == "" {
		return ""
	}
	fields := strings.Fields(obj.Command)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
```

`extractClaudeMessages` rewritten (uuid suffixes by block index; interrupt pre-clean; tool metadata):

```go
func extractClaudeMessages(rec claudeRecord) []models.Message {
	ts, err := time.Parse(time.RFC3339, rec.Timestamp)
	if err != nil {
		ts = time.Now()
	}
	role := rec.Message.Role

	// content as a plain string
	var s string
	if err := json.Unmarshal(rec.Message.Content, &s); err == nil {
		interrupted := strings.Contains(s, interruptMarker)
		text := sync.CleanContent(s)
		if text == "" {
			return nil
		}
		return []models.Message{{Role: role, Content: text, ContentType: classifyText(text), Timestamp: ts,
			UUID: rec.UUID, WasInterrupted: interrupted}}
	}

	// content as an array of blocks
	var blocks []claudeBlock
	if err := json.Unmarshal(rec.Message.Content, &blocks); err != nil {
		return nil
	}
	var out []models.Message
	var textParts []string
	interrupted := false
	for i, b := range blocks {
		switch b.Type {
		case "text":
			if strings.Contains(b.Text, interruptMarker) {
				interrupted = true
			}
			if c := sync.CleanContent(b.Text); c != "" {
				textParts = append(textParts, c)
			}
		case "tool_use":
			if t := SerializeToolInput(b.Name, b.Input); strings.TrimSpace(t) != "" {
				out = append(out, models.Message{Role: role, Content: t, ContentType: "tool", Timestamp: ts,
					UUID:        blockUUID(rec.UUID, "t", i),
					ToolName:    b.Name,
					CommandHead: commandHead(b.Input),
					ToolUseID:   b.ID,
				})
			}
		case "tool_result":
			body := SerializeToolOutput(b.Content)
			if b.IsError != nil && *b.IsError {
				body = "error: " + body
			}
			if strings.TrimSpace(body) != "" {
				out = append(out, models.Message{Role: role, Content: body, ContentType: "tool", Timestamp: ts,
					UUID:      blockUUID(rec.UUID, "r", i),
					ToolUseID: b.ToolUseID,
					IsError:   b.IsError,
				})
			}
		}
	}
	if len(textParts) > 0 {
		text := strings.TrimSpace(strings.Join(textParts, " "))
		out = append([]models.Message{{Role: role, Content: text, ContentType: classifyText(text), Timestamp: ts,
			UUID: rec.UUID, WasInterrupted: interrupted}}, out...)
	}
	return out
}

// blockUUID derives a stable per-block identity from the record uuid.
// Block index is stable in append-only session files.
func blockUUID(recordUUID, kind string, idx int) string {
	if recordUUID == "" {
		return ""
	}
	return fmt.Sprintf("%s#%s%d", recordUUID, kind, idx)
}
```

Cross-record is_error pairing, appended at the end of `Parse` (after the Iterate loop, before `return`):

```go
	// Pair tool_result error signals back onto their tool_use messages.
	// Results usually arrive in a later record; ToolUseID links them.
	useIdx := make(map[string]int)
	for i := range msgs {
		if msgs[i].ToolName != "" && msgs[i].ToolUseID != "" {
			useIdx[msgs[i].ToolUseID] = i
		}
	}
	for i := range msgs {
		if msgs[i].ToolName == "" && msgs[i].ToolUseID != "" && msgs[i].IsError != nil {
			if j, ok := useIdx[msgs[i].ToolUseID]; ok {
				msgs[j].IsError = msgs[i].IsError
			}
		}
	}
```

Add `"fmt"` to imports.

NOTE (behavior change to call out in the report): `b.IsError` became `*bool`; the existing `"error: "` prefix behavior is preserved via `b.IsError != nil && *b.IsError`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestClaudeReaderRichCapture ./internal/readers/`
Expected: PASS

- [ ] **Step 5: Run full readers suite (regressions: existing fixtures must still pass)**

Run: `go test ./internal/readers/ && just check`
Expected: all PASS

- [ ] **Step 6: Report STATUS to orchestrator**

### Task 4: Plumb captured fields through sync into search_items + tool_events

**Files:**
- Modify: `cmd/backscroll/sync_helpers.go:93-102` (IndexedMessage construction)
- Modify: `internal/storage/sync.go:41-96` (SyncFiles: new columns + tool_events rows)
- Test: `internal/storage/toolevents_sync_test.go` (create)

**Interfaces:**
- Consumes: Task 2 fields, Task 3 captured messages.
- Produces: `search_items` rows with `uuid`, `extraction_version`, `was_interrupted` populated; one `tool_events` row per message with `ToolName != ""`.

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestSyncFilesWritesToolEvents(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	files := []IndexedFile{{
		SourcePath: "/p/s.jsonl", Source: "session", Hash: "h1", Project: "proj",
		Messages: []IndexedMessage{
			{Ordinal: 0, Role: "user", Text: "no, otra cosa", UUID: "u1", Timestamp: "2026-01-01T00:00:00Z",
				ContentType: "text", WasInterrupted: true, ExtractionVersion: CurrentExtractionVersion},
			{Ordinal: 1, Role: "assistant", Text: "Bash command=go test", UUID: "u2#t0", Timestamp: "2026-01-01T00:00:01Z",
				ContentType: "tool", ToolName: "Bash", CommandHead: "go", IsError: boolPtr(true),
				ExtractionVersion: CurrentExtractionVersion},
		},
	}}
	if err := db.SyncFiles(files); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var wasInterrupted, extractionVersion int
	if err := db.db.QueryRow(`SELECT was_interrupted, extraction_version FROM search_items WHERE uuid = 'u1'`).
		Scan(&wasInterrupted, &extractionVersion); err != nil {
		t.Fatalf("query u1: %v", err)
	}
	if wasInterrupted != 1 || extractionVersion != CurrentExtractionVersion {
		t.Errorf("u1: was_interrupted=%d extraction_version=%d", wasInterrupted, extractionVersion)
	}

	var toolName, commandHead string
	var isError int
	if err := db.db.QueryRow(`SELECT tool_name, command_head, is_error FROM tool_events WHERE message_uuid = 'u2#t0'`).
		Scan(&toolName, &commandHead, &isError); err != nil {
		t.Fatalf("query tool_events: %v", err)
	}
	if toolName != "Bash" || commandHead != "go" || isError != 1 {
		t.Errorf("tool_events row: %s %s %d", toolName, commandHead, isError)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestSyncFilesWritesToolEvents ./internal/storage/`
Expected: FAIL (columns not written / tool_events empty)

- [ ] **Step 3: Implement**

`cmd/backscroll/sync_helpers.go` — replace the IndexedMessage construction loop body:

```go
			for ordinal, msg := range pf.Records {
				sessionText += msg.Content + "\n"
				indexedMsgs = append(indexedMsgs, storage.IndexedMessage{
					Ordinal:           ordinal,
					Role:              msg.Role,
					Text:              msg.Content,
					UUID:              msg.UUID,
					Timestamp:         msg.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
					ContentType:       msg.ContentType,
					ToolName:          msg.ToolName,
					CommandHead:       msg.CommandHead,
					IsError:           msg.IsError,
					WasInterrupted:    msg.WasInterrupted,
					ExtractionVersion: storage.CurrentExtractionVersion,
				})
			}
```

`internal/storage/sync.go` — in `SyncFiles`, replace the search_items INSERT with the extended column list, and add tool_events maintenance:

```go
		// Delete old tool_events for this source_path (legacy wipe-and-reload;
		// slice 3 narrows this to non-perennial paths only)
		if _, err := tx.Exec("DELETE FROM tool_events WHERE source_path = ?", file.SourcePath); err != nil {
			return fmt.Errorf("delete old tool_events for %s: %w", file.SourcePath, err)
		}

		for _, msg := range file.Messages {
			var uuidVal interface{}
			if msg.UUID != "" {
				uuidVal = msg.UUID
			}
			var isErrVal interface{}
			if msg.IsError != nil {
				isErrVal = *msg.IsError
			}
			_, err := tx.Exec(`
				INSERT OR IGNORE INTO search_items
				(source, source_path, ordinal, role, text, timestamp, uuid, project, content_type, extraction_version, was_interrupted)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			`,
				file.Source, file.SourcePath, msg.Ordinal, msg.Role, msg.Text,
				msg.Timestamp, uuidVal, file.Project, msg.ContentType,
				msg.ExtractionVersion, msg.WasInterrupted,
			)
			if err != nil {
				return fmt.Errorf("insert search_item for %s: %w", file.SourcePath, err)
			}

			if msg.ToolName != "" {
				if _, err := tx.Exec(`
					INSERT OR IGNORE INTO tool_events
					(message_uuid, source_path, ordinal, tool_name, command_head, is_error, exit_code, extraction_version)
					VALUES (?, ?, ?, ?, ?, ?, NULL, ?)
				`, uuidVal, file.SourcePath, msg.Ordinal, msg.ToolName, msg.CommandHead, isErrVal, msg.ExtractionVersion); err != nil {
					return fmt.Errorf("insert tool_event for %s: %w", file.SourcePath, err)
				}
			}
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -run TestSyncFilesWritesToolEvents ./internal/storage/`
Expected: PASS

- [ ] **Step 5: Full suite**

Run: `go test ./... && just check`
Expected: all PASS

- [ ] **Step 6: Report STATUS to orchestrator**

---

## Slice 3 — Perennial sync semantics (F0b)

### Task 5: Append-only upsert for fully-identified session files

**Files:**
- Modify: `internal/storage/sync.go:30-121` (SyncFiles)
- Test: `internal/storage/perennity_test.go` (create)

**Interfaces:**
- Consumes: Tasks 1-4.
- Produces: sessions where EVERY message has a UUID sync without DELETE (ids stable); files with any uuid-less message (Pi today, legacy Claude) keep wipe-and-reload. Exported behavior relied on by Tasks 6-7.

- [ ] **Step 1: Write the failing test**

```go
package storage

import (
	"path/filepath"
	"testing"
)

func sessionFile(hash string, msgs []IndexedMessage) []IndexedFile {
	return []IndexedFile{{SourcePath: "/p/grow.jsonl", Source: "session", Hash: hash, Project: "proj", Messages: msgs}}
}

func TestGrowingSessionKeepsStableIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m0 := IndexedMessage{Ordinal: 0, Role: "user", Text: "hola", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m0})); err != nil {
		t.Fatal(err)
	}

	var idBefore int64
	if err := db.db.QueryRow("SELECT id FROM search_items WHERE uuid='u1'").Scan(&idBefore); err != nil {
		t.Fatal(err)
	}

	// session grows: same first message, one new message
	m1 := IndexedMessage{Ordinal: 1, Role: "assistant", Text: "hola!", UUID: "u2",
		Timestamp: "2026-01-01T00:00:05Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h2", []IndexedMessage{m0, m1})); err != nil {
		t.Fatal(err)
	}

	var idAfter int64
	if err := db.db.QueryRow("SELECT id FROM search_items WHERE uuid='u1'").Scan(&idAfter); err != nil {
		t.Fatal(err)
	}
	if idBefore != idAfter {
		t.Errorf("perennity violated: id changed %d -> %d on re-sync", idBefore, idAfter)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path='/p/grow.jsonl'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("want 2 rows after growth, got %d", n)
	}
}

func TestUUIDLessSessionKeepsLegacyReload(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m := IndexedMessage{Ordinal: 0, Role: "user", Text: "sin uuid",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}
	if err := db.SyncFiles(sessionFile("h2", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE source_path='/p/grow.jsonl'").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("legacy reload must not duplicate: got %d rows", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run 'TestGrowingSessionKeepsStableIDs|TestUUIDLessSessionKeepsLegacyReload' ./internal/storage/`
Expected: TestGrowingSessionKeepsStableIDs FAILS (id changes — today's DELETE+reinsert)

- [ ] **Step 3: Implement**

In `SyncFiles`, replace the unconditional deletes with a perennial/legacy split at the top of the per-file loop:

```go
	for _, file := range files {
		// Perennial path: session files where every message carries a UUID
		// sync append-only — existing rows (and their ids) are never touched.
		// Anything else keeps wipe-and-reload (correct for mutable sources
		// and the only safe option without per-message identity).
		// NOTE: the loop below completes the check — `perennial` is only
		// final after every message's UUID has been inspected.
		perennial := file.Source == "session" && len(file.Messages) > 0
		for _, m := range file.Messages {
			if m.UUID == "" {
				perennial = false
				break
			}
		}

		if !perennial {
			if _, err := tx.Exec("DELETE FROM search_items WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old search_items for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.Exec("DELETE FROM tool_events WHERE source_path = ?", file.SourcePath); err != nil {
				return fmt.Errorf("delete old tool_events for %s: %w", file.SourcePath, err)
			}
		} else {
			// Transition cleanup: rows indexed BEFORE v8 for this same file
			// have uuid NULL; without this one-time delete the uuid-carrying
			// re-parse would duplicate the whole file. Expired files never
			// re-sync, so their legacy rows persist untouched (perennity).
			if _, err := tx.Exec("DELETE FROM search_items WHERE source_path = ? AND uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy rows for %s: %w", file.SourcePath, err)
			}
			if _, err := tx.Exec("DELETE FROM tool_events WHERE source_path = ? AND message_uuid IS NULL", file.SourcePath); err != nil {
				return fmt.Errorf("delete legacy tool_events for %s: %w", file.SourcePath, err)
			}
		}
		// (INSERT OR IGNORE below is the whole upsert: UNIQUE uuid dedupes
		// perennial rows; UNIQUE(source_path, ordinal) dedupes tool_events.)
```

(The INSERT statements from Task 4 stay unchanged — `INSERT OR IGNORE` + UNIQUE constraints make the append-only path work with no further code.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run 'TestGrowingSessionKeepsStableIDs|TestUUIDLessSessionKeepsLegacyReload' ./internal/storage/`
Expected: both PASS

- [ ] **Step 5: Full suite (watch for tests that assumed reload semantics)**

Run: `go test ./... && just check`
Expected: all PASS; if a test assumed DELETE+reinsert for uuid-carrying sessions, fix the TEST expectation, not the perennial behavior — and list it in the report.

- [ ] **Step 6: Report STATUS to orchestrator**

### Task 6: Non-destructive rebuild (FTS re-derivation from DB)

**Files:**
- Modify: `internal/storage/queries.go` (add `RebuildFTS` next to `OptimizeFTS`)
- Modify: `cmd/backscroll/rebuild.go:13-58` (stop purging)
- Test: `internal/storage/perennity_test.go` (extend)

**Interfaces:**
- Consumes: FTS5 external-content tables `messages_fts`, `tool_fts` (migration v4).
- Produces: `func (d *Database) RebuildFTS() error`; `rebuild` command = RebuildFTS + incremental sync; NEVER deletes rows.

- [ ] **Step 1: Write the failing test**

```go
func TestRebuildFTSRestoresIndexWithoutDataLoss(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m := IndexedMessage{Ordinal: 0, Role: "user", Text: "perennial evidence", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}

	if err := db.RebuildFTS(); err != nil {
		t.Fatalf("rebuild fts: %v", err)
	}

	// row survived and is still searchable after re-derivation
	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items").Scan(&n); err != nil || n != 1 {
		t.Fatalf("data loss: n=%d err=%v", n, err)
	}
	var hits int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'perennial'").Scan(&hits); err != nil || hits != 1 {
		t.Fatalf("fts not re-derived: hits=%d err=%v", hits, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestRebuildFTSRestoresIndexWithoutDataLoss ./internal/storage/`
Expected: FAIL ("RebuildFTS undefined")

- [ ] **Step 3: Implement RebuildFTS**

```go
// RebuildFTS re-derives both FTS indexes from search_items using FTS5's
// external-content 'rebuild' command. It never touches search_items rows —
// the DB, not the filesystem, is the source of truth (perennity contract).
func (d *Database) RebuildFTS() error {
	// Single transaction: either both indexes re-derive or neither does —
	// a partial rebuild would leave one index stale and queries inconsistent.
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO messages_fts(messages_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild messages_fts: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO tool_fts(tool_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("rebuild tool_fts: %w", err)
	}
	return tx.Commit()
}
```

- [ ] **Step 4: Rewrite runRebuild (cmd/backscroll/rebuild.go)**

```go
func runRebuild(stdout, stderr io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := storage.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "Re-deriving FTS indexes from database...\n")
	err = db.RebuildFTS()
	if closeErr := db.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("rebuild FTS: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Running incremental sync...\n")
	if err := maybeAutoSync(cfg); err != nil {
		_, _ = fmt.Fprintf(stderr, "warning: sync failed: %v\n", err)
	}
	_, _ = fmt.Fprintf(stdout, "Rebuild complete. No indexed data was deleted (perennity contract).\n")
	return nil
}
```

Also update `newRebuildCmd`'s `Long` text:

```go
		Long: `Rebuild re-derives the FTS search indexes from the database itself and
runs an incremental sync. It never deletes indexed content: sessions whose
source files have expired from disk are preserved (the database is the
perennial event store). Use 'purge' to delete data explicitly.`,
```

- [ ] **Step 5: Run tests**

Run: `go test -run TestRebuildFTSRestoresIndexWithoutDataLoss ./internal/storage/ && go test ./cmd/backscroll/ && just check`
Expected: all PASS (fix any main_test.go expectations on rebuild's stdout text — the old text promised deletion; update the TEST, list it in the report)

- [ ] **Step 6: Report STATUS to orchestrator**

### Task 7: Purge handles satellites + perennity integration test

**Files:**
- Modify: `internal/storage/queries.go:443-523` (Purge)
- Test: `internal/storage/perennity_test.go` (extend)

**Interfaces:**
- Consumes: tool_events (Task 1), Purge (existing).
- Produces: `Purge` deletes matching `tool_events` rows in the same transaction, BEFORE deleting `search_items` (explicit, no CASCADE).

- [ ] **Step 1: Write the failing test**

```go
func TestPurgeDeletesToolEventsExplicitly(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	old := IndexedMessage{Ordinal: 0, Role: "assistant", Text: "Bash command=rm", UUID: "u1",
		Timestamp: "2020-01-01T00:00:00Z", ContentType: "tool", ToolName: "Bash", CommandHead: "rm", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{old})); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Purge("2021-01-01"); err != nil {
		t.Fatalf("purge: %v", err)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM tool_events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("purge must delete satellite tool_events rows, %d remain", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestPurgeDeletesToolEventsExplicitly ./internal/storage/`
Expected: FAIL (tool_events row remains)

- [ ] **Step 3: Implement**

In `Purge`, immediately BEFORE the `DELETE FROM search_items` statement:

```go
	// Delete satellite tool_events for the purged rows first (explicit —
	// perennial tables have no CASCADE lifecycle by design).
	if _, err := tx.Exec(`
		DELETE FROM tool_events
		WHERE (source_path, ordinal) IN (
			SELECT source_path, ordinal FROM search_items WHERE timestamp < ?
		)
	`, beforeStr); err != nil {
		return 0, fmt.Errorf("delete tool_events: %w", err)
	}
```

- [ ] **Step 4: Run tests**

Run: `go test -run TestPurgeDeletesToolEventsExplicitly ./internal/storage/`
Expected: PASS

- [ ] **Step 5: End-to-end perennity test (the cycle's exit criterion)**

```go
func TestSessionSurvivesSourceFileExpiry(t *testing.T) {
	// Simulates: session indexed -> JSONL expires from disk -> rebuild runs.
	// The perennity contract: rows survive both events.
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	m := IndexedMessage{Ordinal: 0, Role: "user", Text: "irreplaceable history", UUID: "u1",
		Timestamp: "2026-01-01T00:00:00Z", ContentType: "text", ExtractionVersion: 1}
	if err := db.SyncFiles(sessionFile("h1", []IndexedMessage{m})); err != nil {
		t.Fatal(err)
	}

	// "File expired": no further syncs mention /p/grow.jsonl. Rebuild runs.
	if err := db.RebuildFTS(); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM search_items WHERE uuid='u1'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("perennity violated: row lost after source expiry + rebuild (n=%d err=%v)", n, err)
	}
}
```

Run: `go test -run TestSessionSurvivesSourceFileExpiry ./internal/storage/`
Expected: PASS

- [ ] **Step 6: Full CI parity**

Run: `just ci`
Expected: build OK, all tests PASS, aggregate coverage ≥85%

- [ ] **Step 7: Report STATUS to orchestrator**

---

## Deferred (explicitly NOT in this cycle)

- Pi/OpenCode per-message ids: `piRecord` parses no id today; Pi/OpenCode files stay on the legacy wipe-and-reload path (safe by Task 5's rule). Verifying whether Pi JSONL carries a usable id (`rg '"id"' <pi-session>.jsonl`) and wiring it is a follow-up slice.
- `exit_code` regex mining from Bash output (F1 scope; column exists, stays NULL).
- CLAUDE.md documentation update for the rebuild contract change — folded into the cycle-close commit by the orchestrator.
- `backscroll export` / backup story (named future slice in the north star).

## Cycle exit checks (orchestrator runs at close)

1. `just ci` green on final HEAD.
2. `go test -run 'TestGrowingSessionKeepsStableIDs|TestSessionSurvivesSourceFileExpiry' ./internal/storage/` — the two perennity invariants, explicitly.
3. Real-DB smoke: `backscroll status` + one `search` against the production DB after a live sync (ids stable, uuid populated for NEW rows: `sqlite3 -readonly ~/.backscroll.db "SELECT COUNT(*) FROM search_items WHERE uuid IS NOT NULL"` > 0).
4. CLAUDE.md updated (rebuild semantics, tool_events table, migration v8 note).
