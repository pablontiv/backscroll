# Searchable Tool Calls — Slice 3 (OpenCode) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make OpenCode tool inputs and outputs searchable in FTS5 by extending `OpenCodeReader` to capture `tool` parts (`state.input` / `state.output`).

**Architecture:** OpenCode already has its own Go reader (`OpenCodeReader`, format `"opencode"`, registered) parsing the SQLite `part` table. This slice extends its parse loop to emit `content_type="tool"` messages for `tool` parts, reusing the Slice 1 `toolfmt` serializer. No new reader, no registration, no preset flip — `opencode.inputs.toml` already uses `format="opencode"`.

**Tech Stack:** Go (stdlib `encoding/json`, `database/sql`), `modernc.org/sqlite`. Tests: stdlib `testing` with temp SQLite DBs.

## Global Constraints

- Per-package statement coverage floor ≥85% (pkcov, enforced pre-push and CI via `just coverage-check`).
- `gofmt` clean and `go vet` clean (`just check`).
- Pure Go, no CGO.
- Conventional Commits (`type(scope): description`); no AI attribution / Co-Authored-By lines.
- This slice does NOT touch Claude, Pi, or delete the declarative engine / `JsonlReader` (Slice 4). It must not change existing OpenCode text-only behavior (existing `OpenCodeReader` tests stay green).

## Scope / context an engineer needs

- `OpenCodeReader` lives in `internal/readers/opencode_reader.go`. It opens the OpenCode SQLite DB read-only and runs a `message JOIN part` query ordered by time, accumulating text parts into one text message per message id, and SKIPS every non-`text` part.
- The reader is ALREADY registered (`cmd/backscroll/sync_helpers.go`) and the shipped `inputs/opencode.inputs.toml` already has `format = "opencode"`. No wiring changes.
- Reuse from Slice 1: `SerializeToolInput(name string, input json.RawMessage) string` and `SerializeToolOutput(content json.RawMessage) string` (`internal/readers/toolfmt.go`).
- **Real OpenCode tool part shape (verified against a live `opencode.db`):**
  ```json
  {"type":"tool","tool":"bash","callID":"call_...","state":{"status":"completed","input":{"command":"...","description":"..."},"output":"...","metadata":{...},"time":{...},"title":"..."}}
  ```
  `state.input` is an object; `state.output` is a string. A single `tool` part holds BOTH the input and the output.
- **IMPORTANT — existing test fixtures use a DIFFERENT, non-real shape:** `createOpenCodeDB` in `internal/readers/opencode_reader_test.go` inserts parts of type `"tool-use"` (with `name`) and `"tool-result"` (with top-level `output`). These do NOT match the real `type:"tool"` schema. They were placeholder shapes. Because they are neither `"text"` nor `"tool"`, they remain skipped after this change — existing assertions (which expect them skipped) stay valid. Do NOT change `createOpenCodeDB`; add new tool tests with their own DB so existing counts are unaffected.
- `models.Message{Role, Content, ContentType string; Timestamp time.Time}`. `normalizeOpenCodeRole(role string) string` already exists.
- Design source of truth: `docs/superpowers/specs/2026-06-29-searchable-tool-calls-reader-per-agent-design.md`.

## File Structure

- Modify: `internal/readers/opencode_reader.go` — extend `partInfoData`, add `toolPartState`, restructure the parse loop to emit tool messages.
- Modify: `internal/readers/opencode_reader_test.go` — add a tool-part DB helper and unit tests.
- Modify: `CLAUDE.md` — content-type classification note for OpenCode.

---

### Task 1: Extend OpenCodeReader to capture `tool` parts

**Files:**
- Modify: `internal/readers/opencode_reader.go` (struct `partInfoData` at lines 50–54; new `toolPartState`; the `var`/`flush`/loop block at lines 81–139)
- Test: `internal/readers/opencode_reader_test.go` (add helper + tests)

**Interfaces:**
- Consumes: `SerializeToolInput`, `SerializeToolOutput` (Slice 1), `normalizeOpenCodeRole` (existing).
- Produces: extended `partInfoData` (adds `Tool string`, `State *toolPartState`); new `type toolPartState struct { Input json.RawMessage; Output json.RawMessage }`. No new exported symbols.

- [ ] **Step 1: Write the failing tests**

Add to `internal/readers/opencode_reader_test.go`:

```go
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
```

(The `contains` helper exists in `jsonl_reader_test.go`, same package; do not redeclare it. `sql`, `json`, `filepath`, `time`, `input_config` are already imported in `opencode_reader_test.go`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/readers/ -run 'TestOpenCodeReader_CapturesToolInputOutput|TestOpenCodeReader_ToolOnlyMessageEmitted' -v`
Expected: FAIL — the current reader skips every non-`text` part, so no `tool` messages are produced (`missing tool input/output message`).

- [ ] **Step 3: Extend the struct definitions**

In `internal/readers/opencode_reader.go`, replace the `partInfoData` struct (lines 50–54) with:

```go
type partInfoData struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Ignored *bool          `json:"ignored"`
	Tool    string         `json:"tool"`
	State   *toolPartState `json:"state"`
}

// toolPartState is the `state` object of an OpenCode `tool` part. A single tool
// part carries both the input (an object) and the output (a string).
type toolPartState struct {
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}
```

- [ ] **Step 4: Restructure the parse loop**

In `internal/readers/opencode_reader.go`, replace the entire block from `var (` (line 81) through the `flush()` that follows the loop (line 139) with:

```go
	var (
		msgs         []models.Message
		currentMsgID string
		currentRole  string
		currentTime  int64
		textParts    []string
		toolMsgs     []models.Message
	)

	flush := func() {
		if len(textParts) > 0 {
			msgs = append(msgs, models.Message{
				Role:        normalizeOpenCodeRole(currentRole),
				Content:     strings.Join(textParts, "\n"),
				ContentType: "text",
				Timestamp:   time.UnixMilli(currentTime),
			})
		}
		msgs = append(msgs, toolMsgs...)
		textParts = nil
		toolMsgs = nil
	}

	for rows.Next() {
		var (
			msgID       string
			sessionID   string
			msgData     string
			timeCreated int64
			partData    string
		)
		if err := rows.Scan(&msgID, &sessionID, &msgData, &timeCreated, &partData); err != nil {
			continue
		}

		var pd partInfoData
		if err := json.Unmarshal([]byte(partData), &pd); err != nil {
			continue
		}

		if msgID != currentMsgID {
			flush()
			currentMsgID = msgID
			var md msgInfoData
			if err := json.Unmarshal([]byte(msgData), &md); err == nil {
				currentRole = md.Role
			} else {
				currentRole = ""
			}
			currentTime = timeCreated
		}

		switch pd.Type {
		case "text":
			if pd.Ignored != nil && *pd.Ignored {
				continue
			}
			if text := strings.TrimSpace(pd.Text); text != "" {
				textParts = append(textParts, text)
			}
		case "tool":
			if pd.State == nil {
				continue
			}
			role := normalizeOpenCodeRole(currentRole)
			ts := time.UnixMilli(currentTime)
			if in := SerializeToolInput(pd.Tool, pd.State.Input); strings.TrimSpace(in) != "" {
				toolMsgs = append(toolMsgs, models.Message{Role: role, Content: in, ContentType: "tool", Timestamp: ts})
			}
			if out := SerializeToolOutput(pd.State.Output); strings.TrimSpace(out) != "" {
				toolMsgs = append(toolMsgs, models.Message{Role: role, Content: out, ContentType: "tool", Timestamp: ts})
			}
		}
	}
	flush()
```

This preserves text behavior (one text message per message id, same accumulation and ordering) and adds tool messages after the text message for each message id. The msgID-change handling now runs for any part type, so messages whose only parts are tools still get their role and timestamp set.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/readers/ -run TestOpenCodeReader -v`
Expected: PASS — both new tool tests AND all existing OpenCodeReader tests (text-only behavior unchanged; the placeholder `tool-use`/`tool-result` parts are still skipped).

- [ ] **Step 6: Build + vet + fmt + full readers suite**

Run: `gofmt -w internal/readers/ && go vet ./internal/readers/ && go test ./internal/readers/`
Expected: PASS, clean.

- [ ] **Step 7: Commit**

```bash
git add internal/readers/opencode_reader.go internal/readers/opencode_reader_test.go
git commit -m "feat(readers): index OpenCode tool input/output via state.input/output"
```

---

### Task 2: Docs

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:** none (documentation only).

- [ ] **Step 1: Update CLAUDE.md**

In the Key Design Decisions "Content-type classification" bullet, extend it so it notes the `opencode` input now indexes tool parts too. The bullet currently reads (after Slices 1–2) approximately:

> - **Content-type classification**: Messages classified as `text`/`code`/`tool` … The `claude` input indexes `tool_use`/`tool_result` … the `pi` input indexes `toolCall.arguments` and `custom`-record results with `content_type='tool'`.

Append to it: `the opencode input indexes tool parts (state.input and state.output) with content_type='tool'.`

In the Module Layout `readers/` line and the Package Layout `internal/readers` row, ensure `OpenCodeReader` is noted as capturing tool input/output (it is already listed; refine the parenthetical if it implies text-only).

- [ ] **Step 2: Verify docs build / pre-push doc gate**

Run: `just check`
Expected: PASS (gofmt/vet unaffected by docs). The pre-push Module/Package Layout gate only triggers on package add/delete — no new package here, so docs need only stay accurate.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: note OpenCode tool input/output indexing"
```

---

## Self-Review

**Spec coverage (Slice 3 rows of the design):**
- OpenCodeReader captures `tool` parts (`state.input` + `state.output`) with `content_type="tool"` → Task 1. ✓
- Reuse `toolfmt` serializer (no new serializer) → Task 1 (`SerializeToolInput`/`SerializeToolOutput`). ✓
- No new reader / registration / preset flip (OpenCode already on `format="opencode"`) → confirmed in scope notes. ✓
- Existing OpenCode text-only behavior preserved → Task 1 Step 5 (existing tests stay green). ✓
- Tool-only messages emitted (message with no text part) → Task 1 `TestOpenCodeReader_ToolOnlyMessageEmitted`. ✓
- Docs → Task 2. ✓
- NOT in this slice: declarative-engine deletion / JsonlReader removal (Slice 4). ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. The loop replacement is given in full, not described. ✓

**Type consistency:** `SerializeToolInput(string, json.RawMessage)`, `SerializeToolOutput(json.RawMessage)` used as defined in Slice 1. New `partInfoData` fields (`Tool`, `State`) and `toolPartState` fields (`Input`, `Output`) are `json.RawMessage` so `SerializeTool*` accept them directly. `normalizeOpenCodeRole`, `msgInfoData`, `flush` reused consistently. `contains` reused from `jsonl_reader_test.go` (not redeclared). ✓

## After Slice 3

All three session agents (Claude, Pi, OpenCode) now index tool inputs and outputs. Slice 4 — retiring the declarative engine (`selector`/`predicate`/`transform`/`ParseDeclarative`), removing `JsonlReader` (unused since Slice 2), and trimming `types.go` — is the final cleanup and can proceed next.
