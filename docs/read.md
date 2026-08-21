---
estado: Completed
---
# Migration from Direct File Reads to Source Path Retrieval

The former direct-read CLI path has been removed from living guidance. Backscroll now has one operational retrieval path:

```text
active manifests -> mandatory startup sync -> perennial SQLite -> database-backed query
```

Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, list, patterns, status, and validate.
Use `--source-path` on search as a filter, paired with query text, for database-backed retrieval scoped to a known input path.

## Database-backed source path lookup

Use the search `--source-path` filter when you know a stored input path, a path fragment, or a session identifier. The filter matches the `search_items.source_path` value stored in SQLite; it narrows a normal text query and does not parse arbitrary files from disk.

```bash
# Exact or glob-style stored input path.
backscroll search --text "query terms" --source-path "/home/user/.claude/projects/example/session.jsonl" --robot
backscroll search --text "query terms" --source-path "*/example/*.jsonl" --robot

# UUID/session-id fragment in an indexed source_path.
backscroll search --text "artifact literal" --source-path "*019e0d38-c437-7565-ba11-5dd57d516744*" --all-projects --json
backscroll search --text "$QUERY" --source session --source-path "*session-id*" --all-projects --json

# Tool activity matching a known term within the selected path.
backscroll search --text "go test" --content-type tool --source-path "*/example/*.jsonl" --json
```

Use `--fields full` and a bounded `--max-tokens` value when drilling into a selected path for agent context:

```bash
backscroll search --text "$QUERY" --source-path "*session-id*" --all-projects --robot --fields full --max-tokens 4000
```

## What changed

- Raw session, plan, and Markdown files are ingestion inputs, not the normal retrieval boundary.
- SQLite is the perennial record. Rows remain queryable after source files expire unless `purge` removes them explicitly.
- `list` is for session/document summaries. It supports project, ordering, limit, offset, JSON, and robot output; it does not support message-level filters such as `--source-path`, `--source`, `--role`, date windows, or `--content-type`.
- `search` is the drill-down surface for known paths, source types, roles, dates, tags, and content types.

## Raw-file boundary

Do not fall back to `cat`, `jq`, Python, or filesystem session hunting for normal retrieval. Those raw-file techniques are reserved for explicitly authorized indexing-bug diagnosis after the database-backed commands and diagnostics have been reported.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Query and any required startup sync completed; results may be empty |
| `1` | Invalid arguments, manifest preflight failure, or database/query failure |
