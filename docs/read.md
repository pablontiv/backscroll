---
estado: Completed
---
# Indexed Path Lookup

Backscroll no longer exposes a public direct-file `read` workflow. User-visible retrieval should come from the SQLite index populated by `backscroll sync` or search autosync.

Use `backscroll search` with `--source-path` when you already know an indexed path, filename fragment, or session UUID embedded in the path.

## CLI Usage

```bash
backscroll inputs validate
backscroll sync

# Exact indexed path
backscroll search "query terms" --source-path "/home/user/.claude/projects/example/session.jsonl" --robot

# Glob-style path pattern
backscroll search "query terms" --source-path "*/example/*.jsonl" --robot

# UUID/session-id fragment in an indexed source_path
backscroll search "query terms" --source sessions --source-path "*019e0d38-c437-7565-ba11-5dd57d516744*" --all-projects --robot

# Exhaustive ordered records without a search term (empty search string)
backscroll search "" --json --indexed-only --all-projects --source-path "*/example/*.jsonl"

# Tool activity — commands, args, outputs, errors
backscroll search "" --json --indexed-only --all-projects --content-type tool
```

The search query still controls the full-text match. `--source-path` narrows those matches to rows whose indexed `search_items.source_path` equals the provided path or matches the provided `*`/SQL `LIKE` pattern.

For deterministic local tooling, use `backscroll list --json` or `backscroll search "" --json` with `--indexed-only` to stream indexed records without full-text ranking. Records are ordered by `source_path`, `ordinal`, `timestamp`, and row id, include schema version plus project/source identifiers, role, content type, timestamp, ordinal, and bounded text, and support filters such as `--project`, `--all-projects`, `--source`, `--source-path`, `--after`, `--before`, `--limit`, and `--indexed-only`.

For tool activity, use `backscroll search "" --json --indexed-only --content-type tool` to retrieve only messages with `content_type='tool'` — serialized tool inputs, outputs, and errors.

## How It Works

- Files are indexed via `backscroll search` auto-sync (or manually via `backscroll rebuild`).
- Each indexed message stores its original `source_path` in SQLite.
- `search --source-path` and `list --source-path` filter over that indexed `source_path`; they do not parse arbitrary files directly.
- Search and list output includes `source_path` in text, JSON, and robot formats; JSON emits one indexed record per line.

This preserves the database as the source of truth and avoids stale direct-file reads.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Search completed (results may be empty) |
| `1` | Error (invalid input manifests, database/query failure) |
