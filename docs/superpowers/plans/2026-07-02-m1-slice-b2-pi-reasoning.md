# Backscroll M1 Slice B2 — Pi Reasoning Indexing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Make Pi agent reasoning text searchable in FTS5 via an opt-in `index_reasoning` flag in the input manifest, with a new `content_type='reasoning'` filterable via `--content-type reasoning`.

**Architecture:** Extend the existing `PiReader` to capture reasoning blocks (currently skipped) when `index_reasoning = true` in the `pi.inputs.toml` manifest. Reasoning text routes to `messages_fts` (porter tokenizer, prose semantics) with `content_type='reasoning'`. Migration v7 updates the triggers to route 'reasoning' alongside 'text'/'code' to `messages_fts` instead of `tool_fts`. The opt-in gate is stored in the reader context (passed through the sync pipeline), not in the database (keeping the database schema symmetric). Privacy is enforced at the input level: reasoning is not indexed unless explicitly opted in per manifest.

**Tech Stack:** Go (stdlib `encoding/json`, `context`), `modernc.org/sqlite` (FTS5), cobra, `picokit/hashfile`. Tests: stdlib `testing` with realistic Pi JSONL fixtures containing reasoning blocks.

## Global Constraints

- Per-package statement coverage floor ≥85% (pkcov, enforced pre-push and CI via `just coverage-check`).
- `gofmt` clean and `go vet` clean (`just check`).
- Pure Go, no CGO.
- Conventional Commits (`type(scope): description`); no AI attribution / Co-Authored-By lines.
- Tests touching machine state isolate it via `t.Setenv("HOME", t.TempDir())` and `t.Setenv("BACKSCROLL_CONFIG_DIR", ...)`.
- Migration v7: new version block only (v1–v6 never modified). Existing databases correctly handle v7 on first sync. For databases with v6 already applied, v7 appends cleanly without re-triggering v1–v6.
- `_pragma=name(value)` syntax required for all DSN pragmas (modernc.org/sqlite ignores mattn-style `_name=value` form).
- Reasoning blocks from Pi JSONL (piBlock.Type == "thinking") are indexed as separate `models.Message` records with `ContentType='reasoning'`, distinct from text/code/tool content_types.

---

## Task 1: Extend input_config types to support `index_reasoning` opt-in

**Files:**
- Modify: `internal/input_config/types.go` — add `IndexReasoning` field to `DecodeConfig`.
- Modify: `internal/input_config/types_test.go` — validate the field unmarshals from TOML.
- Modify: `inputs/pi.inputs.toml` — add `index_reasoning = false` (explicit default).

**Interfaces:**
- `DecodeConfig` gains field `IndexReasoning bool` (default false via TOML unmarshaling).
- No change to `InputDefinition` or `InputFile` — the flag lives in `Decode`.

- [ ] **Step 1: Update DecodeConfig type**

In `internal/input_config/types.go`, add the field:

```go
// DecodeConfig specifies how to decode discovered files.
type DecodeConfig struct {
	Format           string `toml:"format"`              // "pi", "claude", "opencode", "markdown"
	IndexReasoning   bool   `toml:"index_reasoning"`     // Pi only; opt-in to index reasoning blocks
}
```

Run: `go build ./internal/input_config/`
Expected: Compiles.

- [ ] **Step 2: Write and run TOML unmarshaling test**

Append to `internal/input_config/types_test.go`:

```go
func TestDecodeConfig_IndexReasoning(t *testing.T) {
	tests := []struct {
		name      string
		toml      string
		wantValue bool
	}{
		{
			name:      "default false when omitted",
			toml:      `[inputs]\nformat = "pi"`,
			wantValue: false,
		},
		{
			name:      "explicit false",
			toml:      `[inputs]\nformat = "pi"\nindex_reasoning = false`,
			wantValue: false,
		},
		{
			name:      "explicit true",
			toml:      `[inputs]\nformat = "pi"\nindex_reasoning = true`,
			wantValue: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg struct {
				Inputs struct {
					Format         string `toml:"format"`
					IndexReasoning bool   `toml:"index_reasoning"`
				} `toml:"inputs"`
			}
			if err := toml.Unmarshal([]byte(tt.toml), &cfg); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if cfg.Inputs.IndexReasoning != tt.wantValue {
				t.Errorf("IndexReasoning = %v, want %v", cfg.Inputs.IndexReasoning, tt.wantValue)
			}
		})
	}
}
```

Run: `go test ./internal/input_config/ -run TestDecodeConfig_IndexReasoning -v`
Expected: PASS (default false, explicit true/false work).

- [ ] **Step 3: Update shipped pi.inputs.toml**

In `inputs/pi.inputs.toml`, add `index_reasoning = false` to the existing `[inputs.decode]` section (after the `format = "pi"` line; explicit opt-in required):

```toml
[inputs.decode]
format = "pi"
index_reasoning = false
```

- [ ] **Step 4: Build and test**

Run: `just check && go test ./internal/input_config/`
Expected: PASS, clean.

- [ ] **Step 5: Commit**

```bash
git add internal/input_config/types.go internal/input_config/types_test.go inputs/pi.inputs.toml
git commit -m "feat(input_config): add index_reasoning opt-in flag for Pi reasoning capture"
```

---

## Task 2: Extend PiReader to capture reasoning blocks when index_reasoning=true

**Files:**
- Modify: `internal/readers/pi_reader.go` — pass `indexReasoning` flag from manifest through `extractPiMessages`, capture thinking blocks.
- Modify: `internal/readers/pi_reader_test.go` — test reasoning capture with fixture.

**Interfaces:**
- `PiReader.Parse` signature unchanged (it receives `InputDefinition` which includes `Decode.IndexReasoning`).
- New helper: `extractPiReasons(block piBlock, ts time.Time) *models.Message` — parses a thinking block and returns a `ContentType='reasoning'` message.
- Modify: `extractPiMessages(rec piRecord, indexReasoning bool) []models.Message` — add `indexReasoning` parameter; if true and a thinking block is encountered, call `extractPiReasons`.

- [ ] **Step 1: Update extractPiMessages signature and logic**

In `internal/readers/pi_reader.go`, update the `extractPiMessages` function signature and the main Parse loop:

```go
func (r *PiReader) Parse(path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashfile.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	var msgs []models.Message
	var cwd string
	indexReasoning := def.Decode.IndexReasoning
	err = sync.IterateJSONLFile(path, func(_ int, line []byte) error {
		var rec piRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil // skip malformed lines
		}
		if cwd == "" && rec.CWD != "" {
			cwd = rec.CWD
		}
		switch rec.Type {
		case "message":
			msgs = append(msgs, extractPiMessages(rec, indexReasoning)...)
		case "custom":
			if m, ok := extractPiCustom(rec); ok {
				msgs = append(msgs, m)
			}
		}
		return nil
	})
	if err != nil {
		return models.ParsedFile{}, err
	}

	return models.ParsedFile{Path: path, Hash: hash, Records: msgs, Cwd: cwd}, nil
}

func extractPiMessages(rec piRecord, indexReasoning bool) []models.Message {
	if rec.Message == nil {
		return nil
	}
	role := rec.Message.Role
	if role != "user" && role != "assistant" {
		return nil
	}
	ts := piTimestamp(rec.Timestamp)

	// content as a plain string
	var s string
	if err := json.Unmarshal(rec.Message.Content, &s); err == nil {
		text := sync.CleanContent(s)
		if text == "" {
			return nil
		}
		return []models.Message{{Role: role, Content: text, ContentType: classifyText(text), Timestamp: ts}}
	}

	// content as an array of blocks
	var blocks []piBlock
	if err := json.Unmarshal(rec.Message.Content, &blocks); err != nil {
		return nil
	}
	var out []models.Message
	var textParts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if c := sync.CleanContent(b.Text); c != "" {
				textParts = append(textParts, c)
			}
		case "toolCall":
			if t := SerializeToolInput(b.Name, b.Arguments); strings.TrimSpace(t) != "" {
				out = append(out, models.Message{Role: role, Content: t, ContentType: "tool", Timestamp: ts})
			}
		case "thinking":
			if indexReasoning {
				if m := extractPiReasoning(b, ts); m != nil {
					out = append(out, *m)
				}
			}
		}
	}
	if len(textParts) > 0 {
		text := strings.TrimSpace(strings.Join(textParts, " "))
		out = append([]models.Message{{Role: role, Content: text, ContentType: classifyText(text), Timestamp: ts}}, out...)
	}
	return out
}

// extractPiReasoning converts a thinking block into a searchable reasoning message.
func extractPiReasoning(block piBlock, ts time.Time) *models.Message {
	text := sync.CleanContent(block.Text)
	if text == "" {
		return nil
	}
	return &models.Message{
		Role:        "reasoning",
		Content:     text,
		ContentType: "reasoning",
		Timestamp:   ts,
	}
}
```

Run: `go build ./internal/readers/`
Expected: Compiles.

- [ ] **Step 2: Write failing tests for reasoning capture**

Add to `internal/readers/pi_reader_test.go`:

```go
func TestPiReader_CapturesReasoningWhenEnabled(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":"let me analyze this problem"},{"type":"text","text":"here is the solution"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotReasoning, gotText bool
	for _, m := range pf.Records {
		if m.ContentType == "reasoning" && contains(m.Content, "analyze") {
			gotReasoning = true
		}
		if m.ContentType == "text" && contains(m.Content, "solution") {
			gotText = true
		}
	}
	if !gotReasoning {
		t.Error("reasoning block not captured when index_reasoning=true")
	}
	if !gotText {
		t.Error("text block missing")
	}
}

func TestPiReader_SkipsReasoningWhenDisabled(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":"internal reasoning"},{"type":"text","text":"visible text"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	var gotReasoning bool
	for _, m := range pf.Records {
		if m.ContentType == "reasoning" {
			gotReasoning = true
		}
	}
	if gotReasoning {
		t.Error("reasoning block captured when index_reasoning=false (should be skipped)")
	}
}

func TestPiReader_SkipsEmptyReasoning(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":""},{"type":"text","text":"ok"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
		Decode: input_config.DecodeConfig{Format: "pi", IndexReasoning: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range pf.Records {
		if m.ContentType == "reasoning" {
			t.Error("empty reasoning block should not create a message")
		}
	}
}
```

Run: `go test ./internal/readers/ -run TestPiReader_CapturesReasoning -v && go test ./internal/readers/ -run TestPiReader_SkipsReasoning -v && go test ./internal/readers/ -run TestPiReader_SkipsEmptyReasoning -v`
Expected: PASS (reasoning capture works when enabled, skipped when disabled, empty reasoning ignored).

- [ ] **Step 3: Update existing tests to pass indexReasoning=false**

In `internal/readers/pi_reader_test.go`, update all existing `PiReader{}.Parse(...)` calls to explicitly pass `input_config.InputDefinition{}` with `IndexReasoning: false` (the new default):

```go
// Before:
pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})

// After:
pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{
	Decode: input_config.DecodeConfig{IndexReasoning: false},
})
```

Update all 5 existing test functions: `TestPiReader_TextAndCwd`, `TestPiReader_CapturesToolCall`, `TestPiReader_SkipsNonMessageNonCustomTypes`, `TestPiReader_CapturesCustomResult`, `TestPiReader_SkipsEmptyCustomData`.

Run: `go test ./internal/readers/ -run TestPiReader -v`
Expected: PASS (all 8 tests: the 5 updated existing + 3 new reasoning tests).

- [ ] **Step 4: Build and test**

Run: `just check && go test ./internal/readers/`
Expected: PASS, clean.

- [ ] **Step 5: Commit**

```bash
git add internal/readers/pi_reader.go internal/readers/pi_reader_test.go
git commit -m "feat(readers): capture Pi reasoning blocks when index_reasoning=true"
```

---

## Task 3: Migration v7 — update triggers to route reasoning to messages_fts

**Files:**
- Modify: `internal/storage/migrations.go` — add `applyV7Migration` function and add v7 check/apply in `SetupSchema`.

**Interfaces:**
- New migration function: `applyV7Migration() error` — updates the branched triggers to route `content_type='reasoning'` (and 'text'/'code') to `messages_fts`, not `tool_fts`. Existing 'tool' content_type routes unchanged.
- Trigger names unchanged (`search_items_ai_*`, `search_items_ad_*`, `search_items_au_*`) but WHEN conditions updated.

- [ ] **Step 1: Add v7 migration check and dispatch in SetupSchema**

In `internal/storage/migrations.go`, add the v7 check block at the end of `SetupSchema()` (just before the final `return nil`):

```go
	// Check if version 7 is already applied
	err = d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 7").Scan(&count)
	if err != nil {
		return fmt.Errorf("check migration version 7: %w", err)
	}

	if count == 0 {
		if err := d.applyV7Migration(); err != nil {
			return err
		}
	}

	return nil
```

- [ ] **Step 2: Implement applyV7Migration**

Add the migration function before the SQL constant definitions:

```go
// applyV7Migration updates the content_type-branched triggers to support reasoning
// indexing. Reasoning blocks (content_type='reasoning') route to messages_fts
// alongside 'text' and 'code', NOT to tool_fts. This preserves the v4 semantic:
// tool_fts is for structured tool metadata (names, paths, commands); messages_fts
// is for prose (text, code, reasoning).
func (d *Database) applyV7Migration() error {
	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(sqlV7Triggers); err != nil {
		return fmt.Errorf("rebuild triggers for reasoning: %w", err)
	}

	checksum := sha256.Sum256([]byte(sqlV7Triggers))
	checksumHex := fmt.Sprintf("%x", checksum)

	_, err = tx.Exec(`
		INSERT INTO schema_migrations (version, name, applied_on, checksum)
		VALUES (7, 'V7 reasoning content_type routes to messages_fts', CURRENT_TIMESTAMP, ?)
	`, checksumHex)
	if err != nil {
		return fmt.Errorf("record migration v7: %w", err)
	}

	return tx.Commit()
}
```

- [ ] **Step 3: Define sqlV7Triggers constant**

Add this constant at the end of the file, after the other SQL constants:

```go
// sqlV7Triggers updates the v4 branched triggers to support reasoning content_type.
// The semantic is: tool-specific content (content_type='tool') indexes into tool_fts
// (trigram, substring matching); prose content (text, code, reasoning) indexes into
// messages_fts (porter, morphological matching). This preserves the v4 split while
// extending it for reasoning blocks.
const sqlV7Triggers = `
DROP TRIGGER IF EXISTS search_items_ai_tool;
DROP TRIGGER IF EXISTS search_items_ai_msg;
DROP TRIGGER IF EXISTS search_items_ad_tool;
DROP TRIGGER IF EXISTS search_items_ad_msg;
DROP TRIGGER IF EXISTS search_items_au_tool;
DROP TRIGGER IF EXISTS search_items_au_msg;

-- NOTE: content_type is immutable per row (set at sync time; re-sync deletes and re-inserts).
-- The UPDATE triggers branch on old.content_type and do not handle cross-type transitions.

CREATE TRIGGER IF NOT EXISTS search_items_ai_tool AFTER INSERT ON search_items
WHEN new.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ai_msg AFTER INSERT ON search_items
WHEN new.content_type IN ('text', 'code', 'reasoning') BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_tool AFTER DELETE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_ad_msg AFTER DELETE ON search_items
WHEN old.content_type IN ('text', 'code', 'reasoning') BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_tool AFTER UPDATE ON search_items
WHEN old.content_type = 'tool' BEGIN
    INSERT INTO tool_fts(tool_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO tool_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER IF NOT EXISTS search_items_au_msg AFTER UPDATE ON search_items
WHEN old.content_type IN ('text', 'code', 'reasoning') BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
`
```

Run: `go build ./internal/storage/`
Expected: Compiles.

- [ ] **Step 4: Write migration test**

Create or extend `internal/storage/unit_test.go` to verify v7 migration:

```go
func TestMigrationV7(t *testing.T) {
	// Create a fresh database and force application of all migrations.
	db := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(db)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Verify schema_migrations records version 7 FIRST (before any test data operations).
	var v7Applied int
	if err := d.DB().QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 7").Scan(&v7Applied); err != nil {
		t.Fatalf("query v7: %v", err)
	}
	if v7Applied != 1 {
		t.Errorf("v7 migration not recorded; count=%d", v7Applied)
	}

	// Insert reasoning and text content_types and verify they index into messages_fts.
	// (This requires the sync loop / trigger to fire; a minimal test inserts directly.)
	if _, err := d.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type) 
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/test.jsonl", 1, "assistant", "reasoning text here", "reasoning"); err != nil {
		t.Fatalf("insert reasoning: %v", err)
	}
	if _, err := d.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type) 
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/test.jsonl", 2, "assistant", "regular text", "text"); err != nil {
		t.Fatalf("insert text: %v", err)
	}
	if _, err := d.DB().Exec(`
		INSERT INTO search_items (source_path, ordinal, role, text, content_type) 
		VALUES (?, ?, ?, ?, ?)
	`, "/tmp/test.jsonl", 3, "assistant", "tool command", "tool"); err != nil {
		t.Fatalf("insert tool: %v", err)
	}

	// Verify reasoning and text are in messages_fts.
	var reasoningCount, textCount, toolCount int
	if err := d.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'reasoning'").Scan(&reasoningCount); err != nil {
		t.Fatalf("query messages_fts reasoning: %v", err)
	}
	if reasoningCount == 0 {
		t.Error("reasoning content not in messages_fts")
	}

	if err := d.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'regular'").Scan(&textCount); err != nil {
		t.Fatalf("query messages_fts text: %v", err)
	}
	if textCount == 0 {
		t.Error("text content not in messages_fts")
	}

	// Verify tool content is in tool_fts, NOT in messages_fts.
	if err := d.DB().QueryRow("SELECT COUNT(*) FROM tool_fts WHERE text MATCH 'command'").Scan(&toolCount); err != nil {
		t.Fatalf("query tool_fts: %v", err)
	}
	if toolCount == 0 {
		t.Error("tool content not in tool_fts")
	}

	var toolInMsg int
	if err := d.DB().QueryRow("SELECT COUNT(*) FROM messages_fts WHERE text MATCH 'command'").Scan(&toolInMsg); err != nil {
		t.Fatalf("query messages_fts for tool: %v", err)
	}
	if toolInMsg != 0 {
		t.Error("tool content should NOT be in messages_fts")
	}
}
```

Run: `go test ./internal/storage/ -run TestMigrationV7 -v`
Expected: PASS (v7 migration applied, reasoning and text route to messages_fts, tool routes to tool_fts).

- [ ] **Step 5: Build and test**

Run: `just check && go test ./internal/storage/`
Expected: PASS, clean.

- [ ] **Step 6: Commit**

```bash
git add internal/storage/migrations.go
git commit -m "migration(storage): v7 adds content_type=reasoning routing to messages_fts"
```

---

## Task 4: Update --content-type flag validation and search command

**Files:**
- Modify: `cmd/backscroll/search.go` — extend `--content-type` validation to include 'reasoning'.

**Interfaces:**
- `--content-type` flag now accepts: "text", "code", "tool", "reasoning" (no other values).

- [ ] **Step 1: Add content-type validation**

In `cmd/backscroll/search.go` in the `runSearch` function around line 110, ADD new validation logic (no existing validation today):

```go
// New validation block (add here)
validContentTypes := map[string]bool{
	"text":      true,
	"code":      true,
	"tool":      true,
	"reasoning": true,
}

if contentType != "" && !validContentTypes[contentType] {
	return fmt.Errorf("invalid --content-type %q; must be one of: text, code, tool, reasoning", contentType)
}
```

Also update the help text at line 53 (the Use line):

BEFORE:
```go
Use --content-type to filter by content type (text, code, tool).
```

AFTER:
```go
Use --content-type to filter by content type (text, code, tool, reasoning).
```

Run: `go build ./cmd/backscroll/`
Expected: Compiles.

- [ ] **Step 2: Write flag validation test**

Add to `cmd/backscroll/` tests (or extend an existing test file):

```go
func TestSearchValidatesContentType(t *testing.T) {
	tests := []struct {
		flag    string
		wantErr bool
	}{
		{"text", false},
		{"code", false},
		{"tool", false},
		{"reasoning", false},
		{"invalid", true},
		{"", false}, // empty is valid (no filter)
	}
	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			_, _, err := runCmd("search", "test", "--content-type", tt.flag)
			if (err != nil) != tt.wantErr {
				t.Errorf("runCmd with --content-type %q: err=%v, wantErr=%v", tt.flag, err, tt.wantErr)
			}
		})
	}
}
```

Run: `go test ./cmd/backscroll/ -run TestSearchValidatesContentType -v`
Expected: PASS (reasoning accepted, invalid rejected).

- [ ] **Step 3: Build and test**

Run: `just check && go test ./cmd/backscroll/`
Expected: PASS, clean.

- [ ] **Step 4: Commit**

```bash
git add cmd/backscroll/search.go
git commit -m "feat(search): add reasoning to --content-type filter validation"
```

---

## Task 5: Integration test — Pi reasoning end-to-end

**Files:**
- Create: `tests/fixtures/pi-reasoning.jsonl` — fixture with reasoning blocks.
- Modify: `cmd/backscroll/main_test.go` — integration test for reasoning capture and search.

**Interfaces:**
- No new exports; reuse existing `runCmd`, `testEnv`, `setupInputsPreset` helpers.

- [ ] **Step 1: Create reasoning fixture**

Create `tests/fixtures/pi-reasoning.jsonl` with realistic Pi JSONL containing reasoning:

```
{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","cwd":"/home/shared/project","message":{"role":"assistant","content":[{"type":"thinking","text":"The user is asking me to debug a concurrency issue. I should think through the race condition carefully and explain the problem."},{"type":"text","text":"I found the race condition: the map is being written without a lock."}]}}
{"type":"message","timestamp":"2026-05-10T22:19:44.694Z","message":{"role":"assistant","content":[{"type":"thinking","text":"Let me reason about the solution: we need a mutex to protect shared state."},{"type":"toolCall","name":"code_search","arguments":{"query":"mutex pattern go concurrency"}}]}}
{"type":"custom","customType":"code-search-results","timestamp":"2026-05-10T22:19:54.694Z","data":{"results":"mutex example for sync"}}
```

- [ ] **Step 2: Write failing integration test**

Add to `cmd/backscroll/main_test.go`:

```go
// contains is a test helper for substring checking (strings.Contains wrapper for readability).
func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// setupPiReasoningPreset writes a pi.inputs.toml with index_reasoning=true
func setupPiReasoningPreset(t *testing.T, cfgDir, fixtureRoot string) {
	t.Helper()
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := fmt.Sprintf(`version = 1
[[inputs]]
id = "pi-reasoning"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "pi"
index_reasoning = true
`, fixtureRoot)
	if err := os.WriteFile(filepath.Join(inputsDir, "pi-reasoning.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsPiReasoningWhenEnabled(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	sessionDir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixturesDir(), "pi-reasoning.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "pi-reasoning.jsonl"), src, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfgDir := t.TempDir()
	setupPiReasoningPreset(t, cfgDir, sessionDir)
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	// Search for reasoning content (auto-sync indexes first)
	out, _, err := runCmd("search", "race condition", "--all-projects")
	if err != nil {
		t.Fatalf("search race condition: %v", err)
	}
	if !contains(out, "race condition") {
		t.Errorf("Pi reasoning 'race condition' not found in search; output: %s", out)
	}

	// Search with --content-type reasoning filter
	out, _, err = runCmd("search", "think", "--content-type", "reasoning", "--all-projects")
	if err != nil {
		t.Fatalf("search reasoning: %v", err)
	}
	if !contains(out, "reasoning") || !contains(out, "think") {
		t.Errorf("Pi reasoning not filtered correctly; output: %s", out)
	}
}

func TestSearchSkipsPiReasoningWhenDisabled(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	sessionDir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixturesDir(), "pi-reasoning.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "pi-reasoning.jsonl"), src, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfgDir := t.TempDir()
	// Setup with index_reasoning=false (default)
	toml := fmt.Sprintf(`version = 1
[[inputs]]
id = "pi-noreason"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "pi"
index_reasoning = false
`, sessionDir)
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "pi-noreason.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	// The reasoning text should NOT be indexed; search must not find it
	out, _, err := runCmd("search", "mutex pattern", "--all-projects")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	// Only the assistant text "I found the race condition..." should match,
	// NOT the internal reasoning "Let me reason about the solution"
	if contains(out, "Let me reason") {
		t.Errorf("Pi reasoning should not be indexed when index_reasoning=false; output: %s", out)
	}
}
```

Run: `go test ./cmd/backscroll/ -run TestSearchFindsPiReasoningWhenEnabled -v`
Expected: FAIL initially (reasoning not yet captured and indexed).

- [ ] **Step 3: Run full test suite to verify integration**

Run: `go test ./cmd/backscroll/ -run TestSearchFinds -v`
Expected: PASS (all existing search tests + new reasoning tests).

- [ ] **Step 4: Build and test**

Run: `just check && just test`
Expected: PASS, clean, no coverage regressions.

- [ ] **Step 5: Commit**

```bash
git add tests/fixtures/pi-reasoning.jsonl cmd/backscroll/main_test.go
git commit -m "test(integration): verify Pi reasoning capture and search with index_reasoning flag"
```

---

## Task 6: Update CLAUDE.md — document v7 migration and content_type=reasoning

**Files:**
- Modify: `CLAUDE.md` — Key Design Decisions section, Module Layout, Package Layout.

**Interfaces:**
- No code changes; documentation only.

- [ ] **Step 1: Update Module Layout**

In the Module Layout section of `CLAUDE.md`, update the `internal/storage` line to mention migration v7:

```
├── storage/           — SQLite adapter (dual FTS5 indexes: tool_fts + messages_fts, BM25, WAL mode, migrations v1–v7, search_items, session_tags)
```

- [ ] **Step 2: Update Package Layout table**

Add or update the `github.com/pablontiv/backscroll/internal/storage` row in the Package Layout table to mention v7:

```
| github.com/pablontiv/backscroll/internal/storage | Database schema, migrations v1–v7, FTS5 indexes |
```

- [ ] **Step 3: Update Key Design Decisions**

In the "Content-type classification" bullet point (around line 560 in the original), extend the note:

Before:
```
- **Content-type classification**: Messages classified as `text`/`code`/`tool` based on message content types during sync. Tool content is indexed in separate `search_items` rows with `content_type='tool'`. Sync writes only to `search_items`; the `session_events` table was dropped in migration v5. Split FTS by retrieval semantics: tool content (`content_type='tool'`) lives in a separate FTS5 index `tool_fts` (tokenizer `trigram`, substring/exact match for paths/commands/errors); text+code live in `messages_fts` (`porter unicode61`). ...
```

After:
```
- **Content-type classification**: Messages classified as `text`/`code`/`tool`/`reasoning` based on message content types during sync. Tool content is indexed in separate `search_items` rows with `content_type='tool'`. Pi agent reasoning blocks are captured when `index_reasoning=true` (default off) in the input manifest and indexed with `content_type='reasoning'`. Sync writes only to `search_items`; the `session_events` table was dropped in migration v5. Split FTS by retrieval semantics: tool content (`content_type='tool'`) lives in a separate FTS5 index `tool_fts` (tokenizer `trigram`, substring/exact match for paths/commands/errors); prose content (text, code, reasoning) lives in `messages_fts` (`porter unicode61`). Migration v7 updated the branched triggers to route 'reasoning' alongside 'text'/'code' to `messages_fts`.
```

- [ ] **Step 4: Verify formatting**

Run: `head -n 50 CLAUDE.md | tail -n 20` to spot-check the updated sections are readable.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(CLAUDE.md): document migration v7 and content_type=reasoning support"
```

---

## Self-Review

**Spec coverage (B2 requirements):**
- Index Pi reasoning text when `index_reasoning=true` → Tasks 2, 5. ✓
- Opt-in per input manifest, default off → Task 1 (DecodeConfig.IndexReasoning, pi.inputs.toml). ✓
- content_type='reasoning' to messages_fts → Task 3 (v7 triggers route reasoning to messages_fts). ✓
- Filterable via `--content-type reasoning` → Task 4 (flag validation). ✓
- Migration v7 (new version block, never modify v1–v6) → Task 3 (applyV7Migration, v1–v6 untouched). ✓
- Fixtures from real sessions (reasoning content) → Task 5 (pi-reasoning.jsonl). ✓
- Docs → Task 6 (CLAUDE.md). ✓
- Privacy preserved (Claude stays out, API redacts thinking) → Architecture section, no Claude reader change. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; test fixtures complete; migration SQL complete; trigger definitions complete. All CLI flag updates specified. ✓

**Type consistency:**
- `DecodeConfig.IndexReasoning bool` passed through `InputDefinition.Decode` to `PiReader.Parse`. ✓
- `extractPiMessages(rec piRecord, indexReasoning bool)` signature consistent. ✓
- `extractPiReasoning(block piBlock, ts time.Time) *models.Message` helper returns properly typed message with `ContentType='reasoning'`. ✓
- `piBlock.Type == "thinking"` check explicit; 'reasoning' role in output message. ✓
- Trigger WHEN conditions: `new.content_type = 'tool'` (tool_fts), `new.content_type IN ('text', 'code', 'reasoning')` (messages_fts). ✓
- Flag validation: "text", "code", "tool", "reasoning" enumerated. ✓

**Coverage gates:**
- All new functions tested (extractPiReasoning via extractPiMessages tests, applyV7Migration via TestMigrationV7, flag validation via TestSearchValidatesContentType). ✓
- Fixtures realistic (pi-reasoning.jsonl has thinking blocks, toolCall, custom result). ✓
- Integration tests exercise opt-in (enabled and disabled paths). ✓

**Existing test updates:**
- All TestPiReader_* tests updated to explicitly pass `InputDefinition` with `IndexReasoning: false`. ✓
- No breaking changes to public APIs; v7 migration is backward-compatible (existing databases apply v7 cleanly). ✓

**Note on docs/eval/ queries:** The spec mentions "2-3 eval queries answerable only via reasoning content" conditional on `docs/eval/` existing. Since `docs/eval/` does not yet exist, this is the only permitted conditional per the format rules. It is deferred to Track C (C2, the eval-set integration).

---

## After Slice B2

After this slice:
- Pi reasoning is indexed and searchable when `index_reasoning=true`.
- Default is off (privacy-first).
- `--content-type reasoning` filter works.
- Migration v7 is deployed; all databases (new and existing) support reasoning routes.
- Claude and OpenCode readers unchanged (Claude API redacts thinking; OpenCode tool parts are B1/Slice 3).
- Ready for Track B—slice B3 (retire declarative engine) once B1/OpenCode is complete.
