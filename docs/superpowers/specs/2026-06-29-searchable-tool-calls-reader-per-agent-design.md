# Searchable Tool Calls + Reader-Per-Agent — Design

**Date:** 2026-06-29
**Status:** Approved (design phase)

## Problem

Backscroll indexes AI session history into SQLite FTS5, but only indexes `text`
content blocks. Tool calls — the command that ran, the file that was touched, the
output or error that came back — are **not searchable**. This is exactly the
information needed to debug "what happened in that session", and today an LLM
investigating prior sessions has to fall back to parsing raw JSONL because the
index doesn't contain it.

Concretely, the shipped Claude input preset filters content to text only:

```toml
[inputs.content]
include_when = [{ selector = "$.type", op = "eq", value = "text" }]
```

So `tool_use` (what was invoked) and `tool_result` (what came back, including
errors) never reach the index.

## Goal

Make tool **inputs** and **outputs** searchable in FTS5 across all three session
agents (Claude, Pi, OpenCode), so session history is useful for debugging.

While doing this, correct an architectural mismatch the work exposes (below):
move from a shared declarative TOML extraction engine to **one Go reader per
agent**, and retire the now-unjustified declarative engine.

## Why reader-per-agent (architecture decision)

The current dispatch is **by wire format, not by agent**:

```go
// internal/readers/reader.go
func (r *Registry) ForDef(def) {
    format := def.Decode.Format   // "jsonl" | "opencode" | "markdown_sections"
    ...
}
```

- `format=jsonl` → `JsonlReader` (declarative, TOML-driven) ← **claude AND pi**
- `format=opencode` → `OpenCodeReader` (custom Go)
- `format=markdown_sections` → markdown reader (decisions/sources)

The declarative engine exists so two agents that share a wire format (Claude + Pi,
both JSONL-with-content-blocks) can share one reader and differ only in TOML.

Capturing tool calls breaks that premise. The three agents' tool schemas are
genuinely different, as confirmed by inspecting live sessions:

| Agent | tool input | tool output |
|---|---|---|
| Claude | `tool_use` block, `input` object | separate `tool_result` block, `content` (string \| array of `{type:text}`), `is_error` flag |
| Pi | `toolCall` block, `arguments` object | **separate `custom` record** (`customType`, `data`), no `role`, linked by `parentId` |
| OpenCode | `tool` part, `state.input` | same `tool` part, `state.output` + `state.status` (SQLite `part` table) |

Pi's results-as-separate-records and OpenCode's unified-part shape do not fit the
generic block-extraction pipeline without per-agent special cases — at which point
the shared abstraction leaks and stops paying for itself. With each agent owning
its schema in explicit, testable Go, the heavy `record`/`map`/`content`/`text`
TOML config is no longer justified, so it is removed and the declarative engine
retired.

### Live-session grounding (evidence)

Sampled from 6 recent Claude sessions (552 tool results, ~600 tool calls):

- **tool_use input keys vary per tool**: `Bash{command,description,timeout}`,
  `Edit{file_path,old_string,new_string,replace_all}`, `Read{file_path,limit,offset}`,
  `Write{content,file_path}`, MCP `{query,project}`. No single field captures all
  → must serialize the whole input object.
- **tool_result content shapes**: `string` (516), `array[text]` (30),
  `array[tool_reference]` (6). 40 results had `is_error: true`.
- **sizes**: p50 = 259 chars, p90 ≈ 4000, max ≈ 57 KB → truncation needed.

## Target architecture

```
SessionReader (interface, unchanged)
├── ClaudeReader (Go)      text + tool_use.input + tool_result.content + is_error
├── PiReader (Go)          text + toolCall.arguments + custom-record results
├── OpenCodeReader (Go)    text + tool part state.input/output      [extended]
└── markdown_sections      decisions/sources, unchanged

shared helpers (internal/readers):
├── jsonlscan  — scan JSONL lines (incl. >64 KB lines), decode, normalize role,
│                extract cwd/project
└── toolfmt    — serialize a tool input/output value to searchable text, truncate
```

**Dispatch becomes per-agent format.** Each agent manifest declares its own
`decode.format`:

- `claude.inputs.toml` → `decode.format = "claude"` → `ClaudeReader`
- `pi.inputs.toml`     → `decode.format = "pi"`     → `PiReader`
- `opencode.inputs.toml` → `decode.format = "opencode"` → `OpenCodeReader`

The TOML manifest shrinks to: `id`, `source`, `active`, `[inputs.discover]`,
`[inputs.decode] format`. The `[inputs.record]`, `[inputs.map]`,
`[inputs.content]`, `[inputs.text]` blocks are removed.

## Components

### Shared: `jsonlscan`

Low-level JSONL plumbing shared by `ClaudeReader` and `PiReader`:

- Scan lines with a buffer that tolerates oversized lines (>64 KB) — there are
  existing tests guarding the 70 KB-line case (`TestReadPathTailSemantic…`).
- JSON-decode each line into a generic map.
- Normalize role (`user`/`assistant`/…).
- Extract session cwd/project where present (preserve current O18 cwd mapping).

This factors the genuinely-shared mechanics so each reader owns only its block
semantics.

### Shared: `toolfmt`

`Serialize(name string, v any, maxLen int) string` — turns a tool input or output
value into searchable text:

- object → flattened `key=value` pairs (e.g. `command=… description=…`) or compact
  JSON; chosen for searchability, not round-tripping.
- string → as-is.
- array of `{type:text}` → joined `.text`.
- result truncated to `maxLen`.

**Truncation limit is a Go constant** (default ~4000 chars: covers observed p90,
caps the 57 KB outlier). Not TOML-configurable — consistent with retiring the
declarative config. If per-input tuning is needed later, add a minimal
`decode.max_tool_len` field then (YAGNI now).

### `ClaudeReader`

Per JSONL line, over `message.content` blocks:

- `text` block → text content (existing behavior), `content_type = "text"`.
- `tool_use` block → `toolfmt.Serialize(name, input, maxLen)`, `content_type = "tool"`.
- `tool_result` block → content (string, or joined `.text` from array) prefixed
  with error marker when `is_error == true`, `content_type = "tool"`.
- Noise filters currently expressed as TOML `remove` regexes (system-reminder,
  task-notification, command wrappers, etc.) move into Go.

### `PiReader`

Per JSONL line:

- `message` records: `text` block → text; `toolCall` block →
  `toolfmt.Serialize(name, arguments, maxLen)`, `content_type = "tool"`;
  skip `thinking` blocks.
- `custom` records (tool results, e.g. `customType = "web-search-results"`):
  serialize `data` to searchable text, `content_type = "tool"`, role defaulted
  (e.g. `tool`). Indexed for search; `parentId` linkage is not required for FTS.

### `OpenCodeReader` (extended)

Currently filters `pd.Type != "text"` and skips everything else. Extend:

- keep `text` parts.
- `tool` parts: serialize `state.input` and `state.output` (+ `state.status`) via
  `toolfmt`, `content_type = "tool"`.

### Retire declarative engine

Once no reader depends on it (after Claude + Pi move to dedicated readers):

- Delete `internal/input_config/selector.go`, `predicate.go`, `transform.go`.
- Delete `ParseDeclarative`, `ParseDeclarativeWithCwd`, `extractRawContent`,
  `TestFile` from `pipeline.go`.
- Delete `JsonlReader`.
- Trim `types.go`: drop `RecordConfig`, `MapConfig`, `ContentConfig`, `TextConfig`,
  `Predicate`, `RemoveConfig`.
- Keep: `loader.go`, `discover.go`, `compat.go`, and the trimmed `types.go`
  (`InputDefinition`, `DiscoverConfig`, `DecodeConfig`) — still used by
  `status`/`config` commands and all readers for discovery.

## Data flow (unchanged downstream)

```
session files → reader.Discover → SHA-256 dedup → reader.Parse → []Message
   → SyncFiles → search_items (FTS5) + session_events
```

Readers still emit `[]models.Message` with `Content` + `ContentType`. Setting
`ContentType = "tool"` on tool-derived messages enables `--content-type tool`
filtering and keeps tool noise separable from prose at query time.

## Error handling

- Malformed JSONL lines: skipped (current behavior preserved).
- Oversized lines: handled by `jsonlscan` buffer growth, not dropped.
- Missing/odd tool shapes (e.g. `array[tool_reference]`): serialize best-effort or
  skip if no searchable text; never panic.
- OpenCode DB read errors: surfaced as today.

## Testing

Each slice ships with its own tests:

- **Tool input searchable**: a `Bash` command string is found via `search`.
- **Tool output searchable**: a `tool_result` body / error is found via `search`.
- **Error capture**: `is_error` results are searchable (debug value).
- **Noise still filtered**: system-reminder/etc. excluded (Claude).
- **Large-line safety**: oversized JSONL line does not break scanning.
- **content_type=tool**: tool-derived rows filterable via `--content-type tool`.
- **Per-agent fixtures**: Claude blocks, Pi `toolCall` + `custom` records,
  OpenCode `tool` part with `state.input/output`.
- Per-package coverage floor ≥85% maintained (pkcov gate).

## Slicing (delivery)

Each slice is a single PR with its own tests, sized within the ~400-line budget.

| Slice | Scope | Ordering |
|---|---|---|
| **1** | `ClaudeReader` + `jsonlscan` + `toolfmt`; claude → `format="claude"`; shrink `claude.inputs.toml` | First — delivers headline value (Claude is the main debug corpus) |
| **2** | `PiReader` (incl. `custom`-record results); pi → `format="pi"`; shrink `pi.inputs.toml` | After 1 (reuses helpers) |
| **3** | Extend `OpenCodeReader` (`state.input/output`) | Independent; may run parallel to 2 |
| **4** | Retire declarative engine + `JsonlReader`; trim `types.go` | Last — only after no reader uses it (1+2 done) |

Dependency: `JsonlReader`/`ParseDeclarative` must stay alive through slices 1–3
because Pi still uses them until slice 2. Slice 4 is pure deletion + coverage
adjustment.

## Out of scope

- `decisions` / markdown sources (no tool content).
- Per-input configurable truncation (`decode.max_tool_len`) — add later if needed.
- Changing the FTS5 schema, ranking, or query commands.
- Pi `parentId` result→call linkage beyond making result text searchable.

## Docs to update

- `CLAUDE.md`: Module Layout / Package Layout (new reader files, removed
  `input_config` files), External Source Types note, and the content-type
  classification design note (tool blocks now indexed).
- Pre-push hook validates Module/Package Layout sections when packages change.
