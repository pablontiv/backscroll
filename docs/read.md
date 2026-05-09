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
```

The search query still controls the full-text match. `--source-path` narrows those matches to rows whose indexed `search_items.source_path` equals the provided path or matches the provided `*`/SQL `LIKE` pattern.

## How It Works

- `sync` ingests files declared by active manifests under `<config_dir>/backscroll/inputs/*.inputs.toml`.
- Each indexed message stores its original `source_path` in SQLite.
- `search --source-path` filters over that indexed `source_path`; it does not parse arbitrary files directly.
- Search output includes `source_path` in text, JSON, and robot formats.

This preserves the database as the source of truth and avoids stale direct-file reads.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Search completed (results may be empty) |
| `1` | Error (invalid input manifests, database/query failure) |
