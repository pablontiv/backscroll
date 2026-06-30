# Searchable Tool Calls — Slice 2 (PiReader) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Pi agent tool inputs (`toolCall.arguments`) and tool results (separate `custom` records) searchable in FTS5 by introducing a Go-native `PiReader`.

**Architecture:** Reader-per-agent. A new `PiReader` (format `"pi"`) parses Pi JSONL natively, reusing the large-line scanner and noise cleaner in `internal/sync` and the `toolfmt` serializer from Slice 1. Tool-derived messages carry `ContentType = "tool"`. The shipped `pi.inputs.toml` flips to `format = "pi"` and drops the declarative `record/map/content/text` blocks.

**Tech Stack:** Go (stdlib `encoding/json`), `modernc.org/sqlite` (FTS5), cobra, `picokit/hashfile`. Tests: stdlib `testing`.

## Global Constraints

- Per-package statement coverage floor ≥85% (pkcov, enforced pre-push and CI via `just coverage-check`).
- `gofmt` clean and `go vet` clean (`just check`).
- Pure Go, no CGO.
- Conventional Commits (`type(scope): description`); no AI attribution / Co-Authored-By lines.
- Tests touching machine state isolate it via `t.Setenv("HOME", t.TempDir())` and `t.Setenv("BACKSCROLL_CONFIG_DIR", ...)`.
- This slice does NOT touch OpenCode or delete the declarative engine (slices 3–4). `JsonlReader` stays registered. After this slice no input uses format `"jsonl"`, but the reader and the declarative engine remain until Slice 4 removes them together.

## Scope / context an engineer needs

- Slice 1 (merged) established the reader-per-agent pattern. `ClaudeReader` (`internal/readers/claude_reader.go`) is the template: it uses `sync.IterateJSONLFile`, `sync.IsNoiseType`, `sync.CleanContent`, and the `toolfmt` serializer, emits `[]models.Message`, and populates `ParsedFile.Cwd`.
- `toolfmt` (`internal/readers/toolfmt.go`) provides: `SerializeToolInput(name string, input json.RawMessage) string`, `SerializeToolOutput(content json.RawMessage) string`, `const MaxToolTextLen = 4000`. Reuse these unchanged.
- `internal/sync` exports `CleanContent(string) string`, `IsNoiseType(string) bool`, and `IterateJSONLFile(path, func(lineNumber int, line []byte) error) error` (large-line safe).
- `SessionReader` interface (`internal/readers/reader.go`): `Name()`, `Discover(def)`, `Hash(ref)`, `Parse(ref, def)`. Dispatch is by `def.Decode.Format` in `Registry.ForDef`. Readers are registered in `cmd/backscroll/sync_helpers.go`.
- `models.Message{Role, Content, ContentType string; Timestamp time.Time}`, `models.ParsedFile{Path, Hash string; Records []models.Message; Cwd string}`.
- **Pi JSONL schema (verified against live sessions):**
  - Record types: `message`, `custom`, plus non-content types (`session`, `model_change`, `thinking_level_change`, `session_info`) which must be skipped.
  - `message` record: `{type:"message", id, parentId, timestamp (RFC3339, e.g. "2026-05-10T22:19:34.694Z"), cwd, message:{role, content, timestamp}}`. `content` is a string OR an array of blocks.
  - message block types: `text{text}`, `toolCall{name, arguments (object), id}`, `thinking` (skip).
  - `custom` record (tool results): `{type:"custom", customType (e.g. "web-search-results"), data (object), id, parentId, timestamp (RFC3339)}`. `data` shape varies by `customType`.
  - Pi records carry a top-level `cwd` field (e.g. `/home/shared/cartyx`) used for project identification — extract first non-empty, like ClaudeReader.
- **Integration testing note:** `SessionDirsToManifest` (the `BACKSCROLL_SESSION_DIRS` path) was set to `format="claude"` in Slice 1, so it routes to ClaudeReader. To exercise PiReader end-to-end you MUST use a **declarative preset** in the config dir (`<BACKSCROLL_CONFIG_DIR>/backscroll/inputs/pi.inputs.toml` with `format="pi"`), which `ActiveInputs` prioritizes over `SessionDirs`. The existing `setupInputsPreset` helper in `cmd/backscroll/main_test.go` shows the pattern (it writes a claude preset; you write a pi preset).
- Design source of truth: `docs/superpowers/specs/2026-06-29-searchable-tool-calls-reader-per-agent-design.md`.

## File Structure

- Create: `internal/readers/pi_reader.go` — `PiReader` (SessionReader for format `"pi"`).
- Create: `internal/readers/pi_reader_test.go` — unit tests for PiReader.Parse.
- Modify: `internal/readers/claude_reader.go` — rename `classifyClaude` → `classifyText` (shared text/code classifier; PiReader reuses it).
- Modify: `cmd/backscroll/sync_helpers.go` — register `&readers.PiReader{}`.
- Modify: `inputs/pi.inputs.toml` — `format = "pi"`, drop `record/map/content/text` blocks.
- Create: `tests/fixtures/pi-toolcalls.jsonl` — fixture with a `toolCall` message and a `custom` result record.
- Modify: `cmd/backscroll/main_test.go` — integration test: Pi toolCall input + custom result searchable via a pi preset.
- Modify: `CLAUDE.md` — Module Layout + Package Layout (PiReader), content-type note.

---

### Task 1: `PiReader` (message records: text + toolCall + cwd) + shared classifier rename

**Files:**
- Modify: `internal/readers/claude_reader.go` (rename `classifyClaude` → `classifyText`; update its 2 call sites in `extractClaudeMessages`)
- Create: `internal/readers/pi_reader.go`
- Test: `internal/readers/pi_reader_test.go`
- Modify: `cmd/backscroll/sync_helpers.go` (register `&readers.PiReader{}`)

**Interfaces:**
- Consumes: `sync.IterateJSONLFile`, `sync.CleanContent`, `SerializeToolInput`, `classifyText`, `input_config.DiscoverFiles`, `hashfile.HashFile`.
- Produces: `type PiReader struct{}` implementing `SessionReader`; `Name()` returns `"pi"`. Helpers `extractPiMessages(rec piRecord) []models.Message`, `piTimestamp(string) time.Time`, and types `piRecord`/`piMessage`/`piBlock`. `func classifyText(text string) string` (renamed from `classifyClaude`).

- [ ] **Step 1: Rename the shared classifier**

In `internal/readers/claude_reader.go`, rename `func classifyClaude(text string) string` to `func classifyText(text string) string`, and update its two call sites in `extractClaudeMessages` (the string-content path and the text-blocks path) from `classifyClaude(text)` to `classifyText(text)`. No behavior change.

Run: `go build ./internal/readers/ && go test ./internal/readers/ -run TestClaudeReader -v`
Expected: PASS (rename is behavior-preserving).

- [ ] **Step 2: Write the failing PiReader tests**

```go
package readers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func writePiFixture(t *testing.T, lines string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "pi.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPiReader_Name(t *testing.T) {
	if (&PiReader{}).Name() != "pi" {
		t.Error("Name != pi")
	}
}

func TestPiReader_TextAndCwd(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","cwd":"/home/shared/proj","message":{"role":"user","content":"hello pi"}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Cwd != "/home/shared/proj" {
		t.Errorf("Cwd = %q, want /home/shared/proj", pf.Cwd)
	}
	if len(pf.Records) != 1 || pf.Records[0].Content != "hello pi" || pf.Records[0].ContentType != "text" {
		t.Fatalf("records = %+v", pf.Records)
	}
}

func TestPiReader_CapturesToolCall(t *testing.T) {
	line := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"text","text":"searching"},{"type":"toolCall","name":"web_search","arguments":{"queries":["pizzqx_query"]}}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	var gotText, gotTool bool
	for _, m := range pf.Records {
		if m.ContentType == "text" && m.Content == "searching" {
			gotText = true
		}
		if m.ContentType == "tool" && contains(m.Content, "web_search") && contains(m.Content, "pizzqx_query") {
			gotTool = true
		}
	}
	if !gotText {
		t.Error("missing text message")
	}
	if !gotTool {
		t.Error("missing toolCall message")
	}
}

func TestPiReader_SkipsNonMessageNonCustomTypes(t *testing.T) {
	lines := `{"type":"session","timestamp":"2026-05-10T22:19:34.694Z"}` + "\n" +
		`{"type":"model_change","timestamp":"2026-05-10T22:19:34.694Z"}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("records = %d, want 0", len(pf.Records))
	}
}
```

(The `contains` helper already exists in `internal/readers/jsonl_reader_test.go` — same package; do not redeclare it.)

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/readers/ -run TestPiReader -v`
Expected: FAIL — `undefined: PiReader`.

- [ ] **Step 4: Write the implementation**

```go
package readers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/sync"
	"github.com/pablontiv/picokit/hashfile"
)

// PiReader implements SessionReader for Pi agent JSONL sessions.
// It captures text and toolCall inputs from `message` records, and tool
// results from separate `custom` records.
type PiReader struct{}

func (r *PiReader) Name() string { return "pi" }

func (r *PiReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFiles(def.Discover)
}

func (r *PiReader) Hash(path string) (string, error) {
	return hashfile.HashFile(path)
}

type piRecord struct {
	Type       string          `json:"type"`
	Timestamp  string          `json:"timestamp"`
	CWD        string          `json:"cwd"`
	CustomType string          `json:"customType"`
	Data       json.RawMessage `json:"data"`
	Message    *piMessage      `json:"message"`
}

type piMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type piBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Parse reads a Pi JSONL session and returns its messages as a ParsedFile.
// Only `message` records (text + toolCall) and `custom` records (tool results)
// produce messages; other record types are skipped.
func (r *PiReader) Parse(path string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashfile.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	var msgs []models.Message
	var cwd string
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
			msgs = append(msgs, extractPiMessages(rec)...)
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

func piTimestamp(s string) time.Time {
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return ts
}

func extractPiMessages(rec piRecord) []models.Message {
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
			// `thinking` and any other block types are intentionally ignored
		}
	}
	if len(textParts) > 0 {
		text := strings.TrimSpace(strings.Join(textParts, " "))
		out = append([]models.Message{{Role: role, Content: text, ContentType: classifyText(text), Timestamp: ts}}, out...)
	}
	return out
}

// extractPiCustom turns a Pi `custom` record (a tool result) into a searchable
// message. The result has no role of its own, so it is recorded as role "tool".
func extractPiCustom(rec piRecord) (models.Message, bool) {
	body := SerializeToolOutput(rec.Data)
	if strings.TrimSpace(body) == "" {
		return models.Message{}, false
	}
	if rec.CustomType != "" {
		body = rec.CustomType + " " + body
	}
	return models.Message{
		Role:        "tool",
		Content:     body,
		ContentType: "tool",
		Timestamp:   piTimestamp(rec.Timestamp),
	}, true
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/readers/ -run TestPiReader -v`
Expected: PASS (4 tests). `TestPiReader_CapturesToolCall` proves toolCall capture even though the custom-record path is exercised in Task 2.

- [ ] **Step 6: Register the reader**

In `cmd/backscroll/sync_helpers.go`, immediately after `reg.Register(&readers.ClaudeReader{})`, add:

```go
	reg.Register(&readers.PiReader{})
```

- [ ] **Step 7: Build + run readers/cmd tests**

Run: `go build ./... && go test ./internal/readers/ ./cmd/backscroll/`; then `gofmt -w .` and `go vet ./...`.
Expected: PASS, clean.

- [ ] **Step 8: Commit**

```bash
git add internal/readers/pi_reader.go internal/readers/pi_reader_test.go internal/readers/claude_reader.go cmd/backscroll/sync_helpers.go
git commit -m "feat(readers): add PiReader with text, toolCall capture, and cwd"
```

---

### Task 2: PiReader custom-record results

**Files:**
- Test: `internal/readers/pi_reader_test.go` (extend)

**Interfaces:**
- Consumes: `PiReader.Parse`, `extractPiCustom` (Task 1). No new production symbols — Task 1 already implements custom-record capture; this task proves and locks it.

- [ ] **Step 1: Write the failing test**

```go
func TestPiReader_CapturesCustomResult(t *testing.T) {
	lines := `{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","message":{"role":"assistant","content":[{"type":"toolCall","name":"web_search","arguments":{"queries":["q"]}}]}}` + "\n" +
		`{"type":"custom","customType":"web-search-results","timestamp":"2026-05-10T22:19:44.292Z","data":{"queries":[{"query":"q","answer":"pizzqx_answer_token"}]}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, lines), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	var gotResult bool
	for _, m := range pf.Records {
		if m.ContentType == "tool" && contains(m.Content, "pizzqx_answer_token") && contains(m.Content, "web-search-results") {
			gotResult = true
		}
	}
	if !gotResult {
		t.Errorf("custom result not captured; records = %+v", pf.Records)
	}
}

func TestPiReader_SkipsEmptyCustomData(t *testing.T) {
	line := `{"type":"custom","customType":"x","timestamp":"2026-05-10T22:19:44.292Z","data":{}}` + "\n"
	pf, err := (&PiReader{}).Parse(writePiFixture(t, line), input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("empty custom data should yield no message; got %+v", pf.Records)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/readers/ -run TestPiReader_CapturesCustomResult -v && go test ./internal/readers/ -run TestPiReader_SkipsEmptyCustomData -v`
Expected: PASS (capture already implemented in Task 1). If `TestPiReader_CapturesCustomResult` fails, the bug is in Task 1's `extractPiCustom` — fix there. If `TestPiReader_SkipsEmptyCustomData` fails, an empty `data` object serialized to a non-empty string — verify `SerializeToolOutput` on `{}` yields empty; if not, the empty check in `extractPiCustom` needs to treat `{}`-serialization as empty (report it).

- [ ] **Step 3: Commit**

```bash
git add internal/readers/pi_reader_test.go
git commit -m "test(readers): lock PiReader custom-record result capture"
```

---

### Task 3: Wire pi input to PiReader + integration + docs

**Files:**
- Modify: `inputs/pi.inputs.toml`
- Create: `tests/fixtures/pi-toolcalls.jsonl`
- Modify: `cmd/backscroll/main_test.go` (integration test + a pi-preset helper)
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: registered `PiReader` (Task 1), the `search` command, the config-dir preset path (`ActiveInputs` prioritizes `<BACKSCROLL_CONFIG_DIR>/backscroll/inputs/*.inputs.toml` over `SessionDirs`).

- [ ] **Step 1: Create the fixture**

Create `tests/fixtures/pi-toolcalls.jsonl` with exactly these two lines:

```
{"type":"message","timestamp":"2026-05-10T22:19:34.694Z","cwd":"/tmp/piproj","message":{"role":"assistant","content":[{"type":"toolCall","name":"web_search","arguments":{"queries":["pizzqx_marker query"]}}]}}
{"type":"custom","customType":"web-search-results","timestamp":"2026-05-10T22:19:44.292Z","data":{"queries":[{"query":"q","answer":"pizzqx_result_token found"}]}}
```

- [ ] **Step 2: Write the failing integration test + pi-preset helper**

Append to `cmd/backscroll/main_test.go`:

```go
// setupPiPreset writes a minimal pi.inputs.toml (format="pi") pointing at a
// fixture dir into the config dir's inputs/ so auto-sync routes to PiReader.
func setupPiPreset(t *testing.T, cfgDir, fixtureRoot string) {
	t.Helper()
	inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := fmt.Sprintf(`version = 1
[[inputs]]
id = "pi"
source = "session"
active = true
[inputs.discover]
roots = [%q]
include = ["**/*.jsonl"]
[inputs.decode]
format = "pi"
`, fixtureRoot)
	if err := os.WriteFile(filepath.Join(inputsDir, "pi.inputs.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFindsPiToolCallContent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	sessionDir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixturesDir(), "pi-toolcalls.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "pi-toolcalls.jsonl"), src, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfgDir := t.TempDir()
	setupPiPreset(t, cfgDir, sessionDir)
	t.Setenv("BACKSCROLL_CONFIG_DIR", cfgDir)

	// toolCall argument text must be searchable (auto-sync indexes first)
	out, _, err := runCmd("search", "pizzqx_marker", "--all-projects")
	if err != nil {
		t.Fatalf("search toolCall: %v", err)
	}
	if !strings.Contains(out, "pizzqx_marker") {
		t.Errorf("Pi toolCall args not indexed; output: %s", out)
	}

	// custom-record result text must be searchable
	out, _, err = runCmd("search", "pizzqx_result_token", "--all-projects")
	if err != nil {
		t.Fatalf("search custom result: %v", err)
	}
	if !strings.Contains(out, "pizzqx_result_token") {
		t.Errorf("Pi custom result not indexed; output: %s", out)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/backscroll/ -run TestSearchFindsPiToolCallContent -v`
Expected: FAIL — the shipped `pi.inputs.toml` still has `format="jsonl"`. Wait: this test writes its OWN preset with `format="pi"`, so it should route to PiReader once PiReader is registered (it is, from Task 1). If it FAILS at this step, the cause is the env/config wiring, not the preset format — investigate before Step 4. If it already PASSES here, the shipped-preset flip in Step 4 is still required for real-world use; proceed.

Note: this test depends only on the written preset, so it may already PASS after Task 1. That is acceptable — Step 4 is still needed to flip the SHIPPED preset for production.

- [ ] **Step 4: Flip the shipped preset**

Replace the entire contents of `inputs/pi.inputs.toml` with:

```toml
# Shipped Pi input preset.
# Backscroll does not read files from this repository inputs/ directory at runtime;
# copy this file into <config_dir>/backscroll/inputs/ (or set BACKSCROLL_CONFIG_DIR)
# and edit as needed.
#
# Installation:
#   cp inputs/pi.inputs.toml ~/.config/backscroll/inputs/
#   backscroll sync
version = 1

[[inputs]]
id = "pi"
source = "session"
active = true

[inputs.discover]
roots = ["~/.pi/agent/sessions", "~/.pi/agent/sessions-archive"]
include = ["**/*.jsonl"]
exclude = []
follow_symlinks = false

[inputs.decode]
format = "pi"
```

- [ ] **Step 5: Run the integration test + full suite**

Run: `go test ./cmd/backscroll/ -run TestSearchFindsPiToolCallContent -v`
Expected: PASS (both `pizzqx_marker` and `pizzqx_result_token` found).

Run: `just check && just test`
Expected: PASS (no regressions; existing Pi text tests still green).

- [ ] **Step 6: Update CLAUDE.md**

In the Module Layout `readers/` line, add `PiReader` to the list, e.g.:

```
├── readers/           — SessionReader interface, Registry, JsonlReader, ClaudeReader, PiReader (text+toolCall+custom results), OpenCodeReader; toolfmt serializer
```

In the Package Layout `internal/readers` row, mention `PiReader`. In Key Design Decisions, extend the "Content-type classification" bullet to note that the `pi` input indexes `toolCall.arguments` and `custom`-record results with `content_type='tool'`.

- [ ] **Step 7: Commit**

```bash
git add inputs/pi.inputs.toml tests/fixtures/pi-toolcalls.jsonl cmd/backscroll/main_test.go CLAUDE.md
git commit -m "feat(readers): index Pi toolCall and custom-record results via PiReader"
```

---

## Self-Review

**Spec coverage (Slice 2 rows of the design):**
- PiReader captures text + toolCall.arguments + custom-record results → Tasks 1, 2. ✓
- Reuse `toolfmt` serializer + truncation (no new serializer) → Tasks 1, 2 (`SerializeToolInput`/`SerializeToolOutput`). ✓
- Reuse large-line scanner + text cleaner → Task 1 (`sync.IterateJSONLFile`, `sync.CleanContent`). ✓
- `content_type = "tool"` for tool-derived messages → Tasks 1, 2; asserted in tests. ✓
- Per-agent dispatch `format="pi"` + shrink `pi.inputs.toml` → Task 3. ✓
- cwd/project preserved (Pi `cwd` field) → Task 1; asserted `TestPiReader_TextAndCwd`. ✓
- Non-content record types skipped → Task 1; asserted `TestPiReader_SkipsNonMessageNonCustomTypes`. ✓
- Docs → Task 3 Step 6. ✓
- NOT in this slice: OpenCode, declarative-engine deletion (slices 3–4). `JsonlReader` left registered (now unused, removed in Slice 4). ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. Task 3 Step 3's "may already pass" is an explicit, reasoned conditional (the test owns its preset), not a vague instruction. ✓

**Type consistency:** `SerializeToolInput(string, json.RawMessage)`, `SerializeToolOutput(json.RawMessage)`, `classifyText(string) string` used consistently across Tasks 1–2 and the renamed call sites in `claude_reader.go`. `piRecord`/`piMessage`/`piBlock` field names (`Type`, `Timestamp`, `CWD`, `CustomType`, `Data`, `Message`, `Role`, `Content`, `Name`, `Arguments`, `Text`) consistent between Task 1 impl and Task 2 fixtures. `contains` reused from `jsonl_reader_test.go` (not redeclared). ✓

## After Slice 2

After this slice, no input uses format `"jsonl"` (claude→claude, pi→pi, SessionDirs→claude, opencode→opencode, decisions→markdown). `JsonlReader` and the declarative engine become dead code, removed together in Slice 4. Slice 3 (OpenCode tool parts) is independent and may proceed in parallel.
