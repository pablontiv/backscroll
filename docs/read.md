---
estado: Completed
---
# Direct File Reading and Indexed Path Search

Backscroll exposes two distinct retrieval paths:

- `backscroll read` parses a session or plan file directly from disk. It does not read from the SQLite index.
- `backscroll search --source-path` searches rows already stored in SQLite and narrows matches to an indexed path or pattern.

Choose direct reading when the file still exists and you need its contents. Choose indexed search when the database is the source of truth, including sessions whose original files may have expired.

## Direct file reading

```bash
# Structured message output
backscroll read --path ~/.claude/projects/example/session.jsonl

# Positional path is equivalent
backscroll read ~/.claude/projects/example/session.jsonl

# Concise semantic rows, limited to the last 45 rows
backscroll read --path ~/.claude/projects/example/session.jsonl --semantic --tail 45

# Human-readable semantic formatting
backscroll read --path ~/.claude/projects/example/session.jsonl --semantic --pretty
```

Use either the positional path or `--path`, not both. `--tail` and `--pretty` apply to semantic output. Direct reading requires the source file to exist and does not fall back to the perennial index.

## Indexed path search

```bash
# Exact indexed path
backscroll search --text "query terms" --source-path "/home/user/.claude/projects/example/session.jsonl" --robot

# Glob-style path pattern
backscroll search --text "query terms" --source-path "*/example/*.jsonl" --robot

# UUID/session-id fragment in an indexed source_path
backscroll search --text "query terms" --source session --source-path "*019e0d38-c437-7565-ba11-5dd57d516744*" --all-projects --robot

# Tool activity matching a known term within the selected path
backscroll search --text "go test" --content-type tool --source-path "*/example/*.jsonl" --indexed-only --json
```

`search` requires a non-empty query. `--source-path` filters rows whose stored `source_path` equals the value or matches its `*`/SQL `LIKE` pattern. It does not parse arbitrary files.

`backscroll list` lists indexed sessions and supports project, ordering, limit, and offset flags. It does **not** support `--source-path`, source, role, date, or content-type filtering; use `backscroll search` for those filters.

## Auto-sync and snapshot reads

Normal `search` and `list` calls validate active manifests and incrementally index changed inputs before querying. Add `--indexed-only` to read the existing index without discovery or mutation. `backscroll status` and `backscroll validate` are always read-only.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Read or search completed; search results may be empty |
| `1` | Invalid arguments, unreadable file, manifest preflight failure, or database/query failure |
