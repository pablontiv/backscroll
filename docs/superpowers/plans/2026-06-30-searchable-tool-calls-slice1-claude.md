# Searchable Tool Calls — Slice 1 (ClaudeReader) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Claude tool inputs (`tool_use`) and outputs (`tool_result`, including errors) searchable in FTS5 by introducing a Go-native `ClaudeReader`.

**Architecture:** Reader-per-agent. A new `ClaudeReader` (format `"claude"`) parses Claude JSONL natively, reusing the existing large-line scanner and noise filters in `internal/sync`. A new `toolfmt` helper serializes tool input/output values to searchable text and truncates them. Tool-derived messages carry `ContentType = "tool"`. The shipped `claude.inputs.toml` flips to `format = "claude"` and drops the declarative `record/map/content/text` blocks.

**Tech Stack:** Go (stdlib `encoding/json`, `bufio`), `modernc.org/sqlite` (FTS5), cobra, `picokit/hashfile`. Tests: stdlib `testing`.

## Global Constraints

- Per-package statement coverage floor ≥85% (pkcov, enforced pre-push and in CI via `just coverage-check`).
- `gofmt` clean and `go vet` clean (`just check`).
- Pure Go, no CGO.
- Conventional Commits (`type(scope): description`); no AI attribution / Co-Authored-By lines.
- Tests that touch machine state must isolate it via `t.Setenv("HOME", t.TempDir())` and `t.Setenv("BACKSCROLL_CONFIG_DIR", ...)`.
- This slice does NOT touch Pi, OpenCode, or delete the declarative engine (those are slices 2–4). `JsonlReader` and `input_config.ParseDeclarativeWithCwd` stay alive (Pi still uses them).

## Scope / context an engineer needs

- Dispatch is by `def.Decode.Format` in `internal/readers/reader.go` (`Registry.ForDef`), falling back to `"jsonl"`. Readers are registered in `cmd/backscroll/sync_helpers.go` (`reg.Register(&readers.JsonlReader{})` near line 35).
- `SessionReader` interface (`internal/readers/reader.go:13`): `Name() string`, `Discover(def) ([]string, error)`, `Hash(ref) (string, error)`, `Parse(ref, def) (models.ParsedFile, error)`.
- `models.Message{Role, Content, ContentType string; Timestamp time.Time}` and `models.ParsedFile{Path, Hash string; Records []models.Message; Cwd string}` (`internal/models/models.go:24,32`).
- `internal/sync` already provides: `IterateJSONLFile(path, func(lineNumber int, line []byte) error) error` (bufio.Reader-based, large-line safe — `internal/sync/jsonl.go:15`), and unexported `cleanContent(string) string` + `isNoiseType(string) bool` (`internal/sync/sync.go:235,156`). `extractFromBlocks` currently DISCARDS tool content (`tool_use`→flag only; `tool_result`→ignored). We do NOT modify `extractFromBlocks`; ClaudeReader gets its own tool-aware walk.
- Claude record shape (from live sessions): `{type, timestamp, cwd, isMeta, message:{role, content}}`. `content` is a string OR an array of blocks. Block types: `text{text}`, `tool_use{name, input}`, `tool_result{content (string|array[{type:text,text}]), is_error}`.
- `ParsedFile.Cwd` feeds project identification (O18). ClaudeReader must populate it from the first non-empty record `cwd` field.
- Design source of truth: `docs/superpowers/specs/2026-06-29-searchable-tool-calls-reader-per-agent-design.md`.

## File Structure

- Create: `internal/readers/toolfmt.go` — serialize tool input/output to searchable text + truncation. Pure, no I/O.
- Create: `internal/readers/toolfmt_test.go` — unit tests for the serializer.
- Create: `internal/readers/claude_reader.go` — `ClaudeReader` (SessionReader for format `"claude"`).
- Create: `internal/readers/claude_reader_test.go` — unit tests for ClaudeReader.Parse.
- Modify: `internal/sync/sync.go` — export `cleanContent`→`CleanContent`, `isNoiseType`→`IsNoiseType`; update internal callers.
- Modify: `cmd/backscroll/sync_helpers.go` — register `&readers.ClaudeReader{}`.
- Modify: `inputs/claude.inputs.toml` — `format = "claude"`, drop `record/map/content/text` blocks.
- Create: `tests/fixtures/claude-toolcalls.jsonl` — fixture with a `tool_use` bash command and a `tool_result` error.
- Modify: `cmd/backscroll/main_test.go` — integration test: tool command + error searchable.
- Modify: `CLAUDE.md` — Module Layout + Package Layout (new reader files), content-type design note.

---

### Task 1: `toolfmt` serializer + truncation

**Files:**
- Create: `internal/readers/toolfmt.go`
- Test: `internal/readers/toolfmt_test.go`

**Interfaces:**
- Produces: `const MaxToolTextLen = 4000`; `func SerializeToolInput(name string, input json.RawMessage) string`; `func SerializeToolOutput(content json.RawMessage) string`.

- [ ] **Step 1: Write the failing test**

```go
package readers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSerializeToolInput_Object(t *testing.T) {
	in := json.RawMessage(`{"command":"go test ./...","description":"run tests"}`)
	got := SerializeToolInput("bash", in)
	for _, want := range []string{"bash", "command=go test ./...", "description=run tests"} {
		if !strings.Contains(got, want) {
			t.Errorf("SerializeToolInput = %q, missing %q", got, want)
		}
	}
}

func TestSerializeToolOutput_String(t *testing.T) {
	got := SerializeToolOutput(json.RawMessage(`"exit code 1: build failed"`))
	if got != "exit code 1: build failed" {
		t.Errorf("got %q", got)
	}
}

func TestSerializeToolOutput_ArrayText(t *testing.T) {
	got := SerializeToolOutput(json.RawMessage(`[{"type":"text","text":"line one"},{"type":"text","text":"line two"}]`))
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Errorf("got %q", got)
	}
}

func TestSerialize_Truncates(t *testing.T) {
	big := strings.Repeat("x", MaxToolTextLen*2)
	got := SerializeToolOutput(json.RawMessage(`"` + big + `"`))
	if len([]rune(got)) != MaxToolTextLen {
		t.Errorf("truncated len = %d, want %d", len([]rune(got)), MaxToolTextLen)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/readers/ -run TestSerialize -v`
Expected: FAIL — `undefined: SerializeToolInput` / `SerializeToolOutput` / `MaxToolTextLen`.

- [ ] **Step 3: Write minimal implementation**

```go
package readers

import (
	"encoding/json"
	"sort"
	"strings"
)

// MaxToolTextLen caps the searchable text extracted from a single tool input or
// output. Chosen from observed live-session sizes (p90 ~4000 chars); caps the
// rare ~57KB outlier so the FTS index stays lean.
const MaxToolTextLen = 4000

// SerializeToolInput turns a tool name and its input value into searchable text,
// e.g. `bash command=... description=...`. Objects become space-joined key=value
// pairs (keys sorted for determinism); other shapes fall back to compact JSON.
// The result is truncated to MaxToolTextLen runes.
func SerializeToolInput(name string, input json.RawMessage) string {
	out := strings.TrimSpace(name + " " + serializeValue(input))
	return truncateRunes(out, MaxToolTextLen)
}

// SerializeToolOutput turns a tool result value into searchable text. Strings pass
// through; arrays of {type:text} blocks are joined; other shapes become compact
// JSON. The result is truncated to MaxToolTextLen runes.
func SerializeToolOutput(content json.RawMessage) string {
	return truncateRunes(serializeValue(content), MaxToolTextLen)
}

func serializeValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+scalar(obj[k]))
		}
		return strings.Join(parts, " ")
	}
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		parts := make([]string, 0, len(arr))
		for _, el := range arr {
			if t, ok := el["text"]; ok {
				parts = append(parts, scalar(t))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return string(raw)
}

// scalar renders a value as plain text: strings unquoted, everything else compact JSON.
func scalar(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/readers/ -run TestSerialize -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/readers/toolfmt.go internal/readers/toolfmt_test.go
git commit -m "feat(readers): add toolfmt serializer for tool input/output"
```

---

### Task 2: Export noise helpers from `sync`

**Files:**
- Modify: `internal/sync/sync.go` (rename `cleanContent`→`CleanContent` at `:235`, `isNoiseType`→`IsNoiseType` at `:156`, update callers in same file: `extractContent`/`extractFromBlocks` use `cleanContent`; `IsNoiseRecord` uses `isNoiseType`)

**Interfaces:**
- Produces: `func CleanContent(content string) string`; `func IsNoiseType(typ string) bool` (exported; same behavior).

- [ ] **Step 1: Write the failing test**

Append to `internal/sync/noise_test.go`:

```go
func TestExportedNoiseHelpers(t *testing.T) {
	if !IsNoiseType("system-reminder") {
		t.Error("IsNoiseType(system-reminder) = false, want true")
	}
	if IsNoiseType("user") {
		t.Error("IsNoiseType(user) = true, want false")
	}
	got := CleanContent("hello <system-reminder>drop</system-reminder> world")
	if got != "hello world" {
		t.Errorf("CleanContent = %q, want %q", got, "hello world")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/sync/ -run TestExportedNoiseHelpers -v`
Expected: FAIL — `undefined: IsNoiseType` / `CleanContent`.

- [ ] **Step 3: Rename + update callers**

In `internal/sync/sync.go`: rename `func cleanContent` → `func CleanContent` and `func isNoiseType` → `func IsNoiseType`. Update the three internal call sites: in `extractContent` (`cleanContent(strContent)` → `CleanContent(strContent)`), in `extractFromBlocks` (`cleanContent(block.Text)` → `CleanContent(block.Text)`), and in `IsNoiseRecord` (`isNoiseType(r.Type)` → `IsNoiseType(r.Type)`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/sync/ -v`
Expected: PASS (existing sync tests + `TestExportedNoiseHelpers`).

- [ ] **Step 5: Commit**

```bash
git add internal/sync/sync.go internal/sync/noise_test.go
git commit -m "refactor(sync): export CleanContent and IsNoiseType for reuse by readers"
```

---

### Task 3: `ClaudeReader` skeleton (text parity + cwd + registration)

**Files:**
- Create: `internal/readers/claude_reader.go`
- Test: `internal/readers/claude_reader_test.go`
- Modify: `cmd/backscroll/sync_helpers.go` (add `reg.Register(&readers.ClaudeReader{})`)

**Interfaces:**
- Consumes: `sync.IterateJSONLFile`, `sync.IsNoiseType`, `sync.CleanContent` (Task 2); `input_config.DiscoverFiles`; `hashfile.HashFile`.
- Produces: `type ClaudeReader struct{}` implementing `readers.SessionReader`; `Name()` returns `"claude"`. Helper `func extractClaudeMessages(rec claudeRecord) []models.Message`.

- [ ] **Step 1: Write the failing test**

```go
package readers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func writeClaudeFixture(t *testing.T, lines string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClaudeReader_TextAndCwd(t *testing.T) {
	line := `{"type":"user","timestamp":"2024-01-01T00:00:00Z","cwd":"/home/me/proj","message":{"role":"user","content":"hello world"}}` + "\n"
	p := writeClaudeFixture(t, line)
	r := &ClaudeReader{}
	pf, err := r.Parse(p, input_config.InputDefinition{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if pf.Cwd != "/home/me/proj" {
		t.Errorf("Cwd = %q, want /home/me/proj", pf.Cwd)
	}
	if len(pf.Records) != 1 || pf.Records[0].Content != "hello world" {
		t.Fatalf("records = %+v", pf.Records)
	}
	if pf.Records[0].ContentType != "text" {
		t.Errorf("ContentType = %q, want text", pf.Records[0].ContentType)
	}
}

func TestClaudeReader_SkipsNoiseAndMeta(t *testing.T) {
	lines := `{"type":"system-reminder","timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"x"}}` + "\n" +
		`{"type":"user","isMeta":true,"timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"y"}}` + "\n"
	p := writeClaudeFixture(t, lines)
	pf, err := (&ClaudeReader{}).Parse(p, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Records) != 0 {
		t.Errorf("records = %d, want 0", len(pf.Records))
	}
}

func TestClaudeReader_Name(t *testing.T) {
	if (&ClaudeReader{}).Name() != "claude" {
		t.Error("Name != claude")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/readers/ -run TestClaudeReader -v`
Expected: FAIL — `undefined: ClaudeReader`.

- [ ] **Step 3: Write minimal implementation**

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

// ClaudeReader implements SessionReader for Claude Code JSONL sessions.
// It captures text, tool_use inputs, and tool_result outputs (including errors).
type ClaudeReader struct{}

func (r *ClaudeReader) Name() string { return "claude" }

func (r *ClaudeReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFiles(def.Discover)
}

func (r *ClaudeReader) Hash(path string) (string, error) {
	return hashfile.HashFile(path)
}

type claudeRecord struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	CWD       string         `json:"cwd"`
	IsMeta    bool           `json:"isMeta"`
	Message   *claudeMessage `json:"message"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
	IsError bool            `json:"is_error"`
}

// Parse reads a Claude JSONL session and returns its messages as a ParsedFile.
// One record may yield several messages: one for concatenated text plus one per
// tool_use / tool_result block (so each tool call is independently searchable).
func (r *ClaudeReader) Parse(path string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashfile.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	var msgs []models.Message
	var cwd string
	err = sync.IterateJSONLFile(path, func(_ int, line []byte) error {
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil // skip malformed lines
		}
		if cwd == "" && rec.CWD != "" {
			cwd = rec.CWD
		}
		if rec.IsMeta || sync.IsNoiseType(rec.Type) || rec.Message == nil || rec.Message.Role == "" {
			return nil
		}
		msgs = append(msgs, extractClaudeMessages(rec)...)
		return nil
	})
	if err != nil {
		return models.ParsedFile{}, err
	}

	return models.ParsedFile{Path: path, Hash: hash, Records: msgs, Cwd: cwd}, nil
}

func extractClaudeMessages(rec claudeRecord) []models.Message {
	ts, err := time.Parse(time.RFC3339, rec.Timestamp)
	if err != nil {
		ts = time.Now()
	}
	role := rec.Message.Role

	// content as a plain string
	var s string
	if err := json.Unmarshal(rec.Message.Content, &s); err == nil {
		text := sync.CleanContent(s)
		if text == "" {
			return nil
		}
		return []models.Message{{Role: role, Content: text, ContentType: classifyClaude(text), Timestamp: ts}}
	}

	// content as an array of blocks
	var blocks []claudeBlock
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
		case "tool_use":
			if t := SerializeToolInput(b.Name, b.Input); strings.TrimSpace(t) != "" {
				out = append(out, models.Message{Role: role, Content: t, ContentType: "tool", Timestamp: ts})
			}
		case "tool_result":
			body := SerializeToolOutput(b.Content)
			if b.IsError {
				body = "error: " + body
			}
			if strings.TrimSpace(body) != "" {
				out = append(out, models.Message{Role: role, Content: body, ContentType: "tool", Timestamp: ts})
			}
		}
	}
	if len(textParts) > 0 {
		text := strings.TrimSpace(strings.Join(textParts, " "))
		out = append([]models.Message{{Role: role, Content: text, ContentType: classifyClaude(text), Timestamp: ts}}, out...)
	}
	return out
}

func classifyClaude(text string) string {
	if strings.Contains(text, "```") {
		return "code"
	}
	return "text"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/readers/ -run TestClaudeReader -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Register the reader**

In `cmd/backscroll/sync_helpers.go`, immediately after `reg.Register(&readers.JsonlReader{})`, add:

```go
	reg.Register(&readers.ClaudeReader{})
```

- [ ] **Step 6: Run build + full readers/cmd tests**

Run: `go build ./... && go test ./internal/readers/ ./cmd/backscroll/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/readers/claude_reader.go internal/readers/claude_reader_test.go cmd/backscroll/sync_helpers.go
git commit -m "feat(readers): add ClaudeReader with text parity and cwd extraction"
```

---

### Task 4: ClaudeReader tool capture (unit)

**Files:**
- Test: `internal/readers/claude_reader_test.go` (extend)

**Interfaces:**
- Consumes: `ClaudeReader.Parse` (Task 3), `SerializeToolInput`/`SerializeToolOutput` (Task 1). No new production symbols — Task 3's block walk already implements capture; this task proves it and locks behavior.

- [ ] **Step 1: Write the failing test**

```go
func TestClaudeReader_CapturesToolUseAndResult(t *testing.T) {
	lines := `{"type":"assistant","timestamp":"2024-01-01T00:00:00Z","message":{"role":"assistant","content":[{"type":"text","text":"running it"},{"type":"tool_use","name":"Bash","input":{"command":"go test ./...","description":"run tests"}}]}}` + "\n" +
		`{"type":"user","timestamp":"2024-01-01T00:00:01Z","message":{"role":"user","content":[{"type":"tool_result","content":"FAIL: build broken","is_error":true}]}}` + "\n"
	p := writeClaudeFixture(t, lines)
	pf, err := (&ClaudeReader{}).Parse(p, input_config.InputDefinition{})
	if err != nil {
		t.Fatal(err)
	}

	var gotText, gotToolInput, gotToolErr bool
	for _, m := range pf.Records {
		switch {
		case m.ContentType == "text" && m.Content == "running it":
			gotText = true
		case m.ContentType == "tool" && contains(m.Content, "command=go test ./...") && contains(m.Content, "Bash"):
			gotToolInput = true
		case m.ContentType == "tool" && contains(m.Content, "error:") && contains(m.Content, "FAIL: build broken"):
			gotToolErr = true
		}
	}
	if !gotText {
		t.Error("missing text message")
	}
	if !gotToolInput {
		t.Error("missing tool_use input message")
	}
	if !gotToolErr {
		t.Error("missing tool_result error message")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
```

Add `"strings"` to the test file imports if not already present.

- [ ] **Step 2: Run test to verify it passes (capture already implemented in Task 3)**

Run: `go test ./internal/readers/ -run TestClaudeReader_CapturesToolUseAndResult -v`
Expected: PASS. (If FAIL, the bug is in Task 3's `extractClaudeMessages` — fix there.)

- [ ] **Step 3: Commit**

```bash
git add internal/readers/claude_reader_test.go
git commit -m "test(readers): lock ClaudeReader tool_use/tool_result capture"
```

---

### Task 5: Wire claude input to ClaudeReader + integration + docs

**Files:**
- Modify: `inputs/claude.inputs.toml`
- Create: `tests/fixtures/claude-toolcalls.jsonl`
- Modify: `cmd/backscroll/main_test.go` (add integration test)
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: registered `ClaudeReader` (Task 3), the `search` command, `BACKSCROLL_SESSION_DIRS` auto-sync path.

- [ ] **Step 1: Create the fixture**

Create `tests/fixtures/claude-toolcalls.jsonl` with exactly these two lines:

```
{"type":"assistant","timestamp":"2024-01-01T00:00:00Z","cwd":"/tmp/proj","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"grep zzqx_marker file.txt","description":"search marker"}}]}}
{"type":"user","timestamp":"2024-01-01T00:00:01Z","cwd":"/tmp/proj","message":{"role":"user","content":[{"type":"tool_result","content":"zzqx_error_token: no such file","is_error":true}]}}
```

- [ ] **Step 2: Write the failing integration test**

Append to `cmd/backscroll/main_test.go`:

```go
func TestSearchFindsToolCallContent(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	sessionDir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixturesDir(), "claude-toolcalls.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "claude-toolcalls.jsonl"), src, 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	t.Setenv("BACKSCROLL_SESSION_DIRS", sessionDir)

	// tool_use command text must be searchable (auto-sync indexes first)
	out, _, err := runCmd("search", "zzqx_marker", "--all-projects")
	if err != nil {
		t.Fatalf("search tool_use: %v", err)
	}
	if !strings.Contains(out, "zzqx_marker") {
		t.Errorf("tool_use command not indexed; output: %s", out)
	}

	// tool_result error text must be searchable
	out, _, err = runCmd("search", "zzqx_error_token", "--all-projects")
	if err != nil {
		t.Fatalf("search tool_result: %v", err)
	}
	if !strings.Contains(out, "zzqx_error_token") {
		t.Errorf("tool_result error not indexed; output: %s", out)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/backscroll/ -run TestSearchFindsToolCallContent -v`
Expected: FAIL — the default `BACKSCROLL_SESSION_DIRS` path uses the auto-discovered legacy preset (still `jsonl`/declarative or text-only `ParseSessions`), so tool text is absent.

- [ ] **Step 4: Flip the shipped preset to ClaudeReader**

Replace the entire contents of `inputs/claude.inputs.toml` with:

```toml
# Shipped Claude input preset.
# Backscroll does not read files from this repository inputs/ directory at runtime;
# copy this file into <config_dir>/backscroll/inputs/ (or set BACKSCROLL_CONFIG_DIR)
# and edit as needed.
version = 1

[[inputs]]
id = "claude"
source = "session"
active = true

[inputs.discover]
roots = ["~/.claude/projects"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]
follow_symlinks = false

[inputs.decode]
format = "claude"
```

- [ ] **Step 5: Verify the auto-sync path uses ClaudeReader**

The auto-sync default builds a manifest from `BACKSCROLL_SESSION_DIRS`. Confirm that path resolves to the `claude` reader. Inspect `internal/input_config/compat.go` `SessionDirsToManifest`: if it sets `Decode.Format` to `""` or `"jsonl"`, change it to `"claude"` so env-var-driven syncing of `~/.claude` sessions uses ClaudeReader. (If it already routes via the loaded preset rather than a synthesized manifest, no change is needed — the preset from Step 4 governs.)

Run: `go test ./cmd/backscroll/ -run TestSearchFindsToolCallContent -v`
Expected: PASS.

- [ ] **Step 6: Run the full suite + checks**

Run: `just check && just test`
Expected: PASS (no regressions; existing Claude text tests still green).

- [ ] **Step 7: Update CLAUDE.md**

In the `internal/` tree (Module Layout) under `readers/`, update the description to note the new files, e.g.:

```
├── readers/           — SessionReader interface, Registry, JsonlReader, ClaudeReader (text+tool_use+tool_result), OpenCodeReader; toolfmt serializer
```

In the Package Layout table, the `internal/readers` row already exists — extend its description to mention `ClaudeReader` and `toolfmt`. In Key Design Decisions, update the "Content-type classification" bullet to note that `tool_use`/`tool_result` content is now indexed with `content_type='tool'` for the `claude` input.

- [ ] **Step 8: Commit**

```bash
git add inputs/claude.inputs.toml tests/fixtures/claude-toolcalls.jsonl cmd/backscroll/main_test.go CLAUDE.md internal/input_config/compat.go
git commit -m "feat(readers): index Claude tool_use/tool_result content via ClaudeReader"
```

---

## Self-Review

**Spec coverage (Slice 1 rows of the design):**
- ClaudeReader captures text + tool_use.input + tool_result.content + is_error → Tasks 3, 4. ✓
- Shared `toolfmt` serializer + truncation const → Task 1. ✓
- Reuse large-line-safe scanner + noise filters → Tasks 2, 3 (via `sync.IterateJSONLFile`, `sync.IsNoiseType`, `sync.CleanContent`). ✓
- `content_type = "tool"` for tool-derived messages → Task 3 implementation; asserted Task 4. ✓
- Per-agent dispatch `format="claude"` + shrink `claude.inputs.toml` → Task 5. ✓
- cwd/O18 preserved → Task 3 (`ParsedFile.Cwd` from record `cwd`); asserted `TestClaudeReader_TextAndCwd`. ✓
- Docs (CLAUDE.md layout / content-type note) → Task 5 Step 7. ✓
- NOT in this slice: Pi, OpenCode, declarative-engine deletion (slices 2–4). `JsonlReader` intentionally left registered for Pi. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; commands have expected output. Task 5 Step 5 is conditional (inspect-then-maybe-edit) but states the exact file, function, and the concrete change — not a vague "handle config". ✓

**Type consistency:** `SerializeToolInput(string, json.RawMessage)`, `SerializeToolOutput(json.RawMessage)`, `MaxToolTextLen` used identically in Tasks 1, 3, 4. `claudeRecord`/`claudeMessage`/`claudeBlock` field names (`Input`, `Content`, `IsError`, `Name`, `Text`, `Type`) consistent between Task 3 impl and Task 4 fixtures. `sync.CleanContent`/`sync.IsNoiseType` names match Task 2 exports. ✓

## After Slice 1

Slices 2 (PiReader), 3 (OpenCode tool parts), and 4 (retire declarative engine + JsonlReader) get their own plans, written after Slice 1 merges. Slice 4 can only land once Pi has moved off `JsonlReader`/`ParseDeclarativeWithCwd` (Slice 2).
