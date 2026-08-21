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

Human-readable output with terminal bold for match highlights. Each result shows the session path, relevance score, and a snippet:

```
---
[SESSION] ~/.claude/projects/abc/sessions/session.jsonl (Score: 12.34)
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
    "ProjectPath": "/Users/Shared/harness/backscroll",
    "Score": 12.34,
    "Tags": null,
    "ContentType": "text",
    "Rank": 1
  }
]
```

Current full-mode fields are exactly: `Source`, `Role`, `Content`, `FilePath`, `Timestamp`, `SessionID`, `ProjectPath`, `Score`, `Tags`, `ContentType`, and `Rank`. Only `--fields minimal` uses the snake_case payload (`source_path`, `snippet`, `score`, `role`, `timestamp`).

### Robot

Robot mode on search emits deterministic `result_N_field=value` lines:

```
result_0_source=session
result_0_role=assistant
result_0_filepath=/home/user/.claude/projects/example/session.jsonl
result_0_content=matched content with escaped newlines
result_0_score=12.34
result_0_rank=1
```

No ANSI escape codes. Search robot string values escape backslash as `\\`, carriage return as `\r`, and newline as `\n`, keeping each field on one line for context windows.

## Token Limiting

The `--max-tokens` flag applies an approximate token limit (characters / 4) to the total output. Once the limit is reached, no more results are emitted. This is useful when feeding results into context-limited tools.

```bash
backscroll search "decisions" --robot --max-tokens 4000
backscroll search --text "$QUERY" --source-path "$SOURCE_PATH" --robot --fields full --max-tokens 4000
```

The limit is approximate — it will not truncate a result mid-output, but will stop before starting a result that would exceed the budget.

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
