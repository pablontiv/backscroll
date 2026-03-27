---
estado: Completed
---
# Search Engine

`backscroll search` performs full-text search across all indexed sessions using BM25 relevance ranking. Results include highlighted snippets showing where the query matched.

## CLI Usage

```bash
backscroll search "migration plan"
backscroll search "error handling" --project "backscroll"
backscroll search "architecture" --json
backscroll search "deployment" --robot --max-tokens 2000
backscroll search "refactor" --fields full
```

### Flags

| Flag | Description |
|------|-------------|
| `--project <NAME>` | Filter results to a specific project |
| `--json` | Output as JSON lines (one object per result) |
| `--robot` | Output as compact tab-separated format |
| `--fields minimal\|full` | Field set to include (default: `minimal`) |
| `--max-tokens <N>` | Approximate token limit for total output |

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

One JSON object per line. With `--fields minimal`:

```json
{"source_path": "~/.claude/.../session.jsonl", "snippet": "...matched text...", "score": 12.34}
```

With `--fields full`, includes the complete message text alongside the snippet:

```json
{"source_path": "...", "text": "full message content", "match_snippet": "...matched text...", "score": 12.34}
```

### Robot

Compact tab-separated format designed for LLM consumption. Each line contains three fields separated by tabs:

```
source_path\tscore\tsnippet
```

No ANSI escape codes. No headers. Minimal overhead — suitable for piping into context windows.

## Token Limiting

The `--max-tokens` flag applies an approximate token limit (characters / 4) to the total output. Once the limit is reached, no more results are emitted. This is useful when feeding results into context-limited tools.

```bash
backscroll search "decisions" --robot --max-tokens 4000
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
