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

# Exhaustive ordered records without a search term
backscroll sessions query --jsonl --all-projects --source-path "*/example/*.jsonl"

# Normalized audit events without auto-sync
backscroll events query --jsonl --indexed-only --all-projects --event-type command
```

The search query still controls the full-text match. `--source-path` narrows those matches to rows whose indexed `search_items.source_path` equals the provided path or matches the provided `*`/SQL `LIKE` pattern.

For deterministic local tooling, `backscroll sessions query --jsonl` streams indexed records without full-text ranking. Records are ordered by `source_path`, `ordinal`, `timestamp`, and row id, include schema version plus project/source identifiers, role, content type, timestamp, ordinal, and bounded text, and support filters such as `--project`, `--all-projects`, `--source`, `--source-path`, `--after`, `--before`, `--limit`, and `--indexed-only`.

For audit consumers, `backscroll events query --jsonl` streams normalized `session_events` with message, tool_call, tool_result, command, error, metadata, or other event types. It supports the same scope filters plus `--event-type`; `--indexed-only` opens the existing SQLite snapshot read-only and does not run input discovery/sync.

## How It Works

- `sync` ingests files declared by active manifests under `<config_dir>/backscroll/inputs/*.inputs.toml`.
- Each indexed message stores its original `source_path` in SQLite.
- `search --source-path` and `sessions query --source-path` filter over that indexed `source_path`; they do not parse arbitrary files directly.
- Search output includes `source_path` in text, JSON, and robot formats; `sessions query --jsonl` emits one indexed record per line.

This preserves the database as the source of truth and avoids stale direct-file reads.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Search completed (results may be empty) |
| `1` | Error (invalid input manifests, database/query failure) |
