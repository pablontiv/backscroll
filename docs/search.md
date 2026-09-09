---
estado: Completed
---
# Search Engine

The search command performs full-text search across all indexed sessions using BM25 relevance ranking. Results include highlighted snippets showing where the query matched. `--source-path` is a filter: every executable search example must include positional query text or `--text <query>`.

## CLI Usage

```bash
backscroll search "migration plan"
backscroll search "error handling" --project "backscroll"
backscroll search "architecture" --json
backscroll search "deployment" --robot --max-tokens 2000
backscroll search "refactor" --fields full
backscroll search "artifact literal" --source-path "*/session.jsonl" --robot
```

### Flags

| Flag | Description |
|------|-------------|
| `--project <NAME>` | Filter results to a specific project |
| `--json` | Output as a JSON array |
| `--robot` | Output compact `result_N_field=value` lines |
| `--fields minimal\|full` | Field set to include (default: `minimal`) |
| `--max-tokens <N>` | Approximate token limit for total output |
| `--source-path <PATH_OR_PATTERN>` | Filter a normal text query by indexed `source_path`; exact paths or `*`/SQL `LIKE` patterns |

## Output Formats

### Text (default)

Human-readable output with terminal bold for match highlights. Each result uses the exact text-layout envelope emitted by the CLI:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Rank: 1 | Source: session | Role: assistant | Score: 12.34
Path: /home/user/.claude/projects/backscroll/sessions/abc123/session.jsonl
...the migration plan involves three phases...
```

Match markers (`>>>` and `<<<` in the raw snippet) are rendered as bold text in the terminal.

### JSON

`--json` emits one JSON array. With `--fields minimal`:

```json
[
  {"source_path": "~/.claude/.../session.jsonl", "snippet": "...matched text...", "score": 12.34, "role": "assistant", "timestamp": "2026-08-20T12:34:56Z"}
]
```

With `--fields full`, the array encodes `models.SearchResult` without JSON tags, so keys are emitted in the current Go field names (PascalCase), not snake_case:

```json
[
  {
    "Source": "session",
    "Role": "assistant",
    "Content": "...matched text...",
    "FilePath": "~/.claude/.../session.jsonl",
    "Timestamp": "2026-08-20T12:34:56Z",
    "SessionID": "",
    "ProjectPath": "backscroll",
    "Score": 12.34,
    "Tags": null,
    "ContentType": "text",
    "Rank": 1
  }
]
```

Current full-mode fields are exactly: `Source`, `Role`, `Content`, `FilePath`, `Timestamp`, `SessionID`, `ProjectPath`, `Score`, `Tags`, `ContentType`, and `Rank`. `ProjectPath` is a legacy field name; its value is the project identifier (for example `backscroll` or `myproj`), not a filesystem path. Only `--fields minimal` uses the snake_case payload (`source_path`, `snippet`, `score`, `role`, `timestamp`).

### Robot

Robot mode emits deterministic `result_N_field=value` lines. Like JSON,
`--fields minimal` is the default and uses the bounded search snippet:

```
result_0_filepath=/home/user/.claude/projects/example/session.jsonl
result_0_content=bounded matched snippet
result_0_score=12.34
result_0_role=assistant
result_0_timestamp=2026-08-20T12:34:56Z
```

Use `--fields full` when the consumer needs the complete indexed content and
metadata:

```
result_0_source=session
result_0_role=assistant
result_0_filepath=/home/user/.claude/projects/example/session.jsonl
result_0_content=complete content with escaped newlines
result_0_project=backscroll
result_0_content_type=text
result_0_timestamp=2026-08-20T12:34:56Z
result_0_score=12.34
result_0_rank=1
```

No ANSI escape codes. Search robot string values escape backslash as `\\`, carriage return as `\r`, and newline as `\n`, keeping each field on one line for context windows.

## Token Limiting

The `--max-tokens` flag applies Picokit's approximate token estimator (word count
multiplied by 1.3) to the total output. Once the next complete result would
exceed the limit, search stops and emits a parseable omission record when that
record fits the remaining budget:

```
result_3_truncated=true
result_3_omitted=7
```

For robot output, the estimator is applied once to the complete escaped payload,
including all fields and the omission record; estimates rounded independently per
line or per result are not added together. Whole trailing results may be removed
to make room for the omission record. If even that record cannot fit, stdout is
empty. `--max-tokens 0` keeps output unlimited.

This is useful when feeding results into context-limited tools.

```bash
backscroll search "decisions" --robot --max-tokens 4000
backscroll search --text "$QUERY" --source-path "$SOURCE_PATH" --robot --fields full --max-tokens 4000
```

The limit is approximate — it will not truncate a result mid-output, but will stop before starting a result that would exceed the budget.

## Query-Echo Handling

Unfiltered search removes direct `backscroll search` tool invocations before
merging tool and prose rankings. This prevents a retrieval command from
outranking the historical prose it is trying to recover merely because the
command repeats every query term.

The filter applies only to the canonical direct Bash serialization
`Bash command=backscroll search ...` (tool-name matching is case-insensitive).
Explicit `--content-type tool` searches still return those commands. Absolute
paths and wrappers such as `/path/backscroll search`, `env backscroll search`,
and `bash -lc "backscroll search ..."` remain ordinary tool results. Text that
mentions Backscroll and unrelated tool commands are unchanged. There is no
opt-in flag or shell parsing.

## Query Sanitization

User queries are automatically sanitized before being passed to the FTS5 engine:

1. **Dynamic stopword removal** — High-frequency terms (appearing in >50% of documents) are automatically filtered out. These stopwords are computed during `sync` and stored in a `dynamic_stopwords` table, adapting to the corpus without hardcoded dictionaries.
2. **Literal quoting** — Remaining tokens are wrapped in double quotes so special characters (hyphens, colons, parentheses, FTS5 operators like `AND`/`OR`/`NOT`) are treated as literal search terms.
3. **Prefix matching** — Each token gets an FTS5 prefix `*` suffix, enabling substring matching (e.g., "crash" matches "crashloopbackoff").

If all tokens in a query are stopwords, the original query is used unfiltered as a fallback. The FTS5 tokenizer (`porter unicode61`) provides stemming on top of these features.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Search completed (results may be empty) |
| `1` | Error (database not found, query failure) |
