# Downstream audit integration contract

Backscroll is the corpus and structured-query layer. Downstream tools such as a session-audit runner own deterministic findings, thresholds, redaction policy, report rendering, and any ADR/KE/backlog creation.

## Deterministic audit flow

Use an explicit snapshot boundary before auditing with `--indexed-only`:

```bash
# 1. Audit preflight against the existing SQLite snapshot (read-only, no sync)
backscroll status --json --indexed-only

# 2. Scope discovery from indexed items and projects
backscroll list --json --indexed-only --all-projects --limit 0

# 3. Complete message records for deterministic consumers
backscroll search "" --json --indexed-only --all-projects --limit 0

# 4. Tool activity (commands, errors, outputs) — narrowly searchable
backscroll search "" --json --indexed-only --content-type tool --all-projects --limit 0
```

`--indexed-only` opens the existing SQLite index read-only and does not run input discovery or sync. Use it for repeatable audit reads. Commands that require an existing index fail or report `index.usable = false` instead of silently creating a new corpus boundary.

## Status JSON

`backscroll status --json --indexed-only` emits one versioned JSON document:

```json
{
  "version": 1,
  "database": { "path": "/home/user/.backscroll.db", "exists": true },
  "inputs": { "active_count": 1, "inputs": [{ "id": "claude", "source": "session", "active": true }] },
  "index": { "usable": true, "files": 42, "messages": 9001, "projects": 3, "last_sync": "2026-05-11T..." },
  "projects": [{ "project": "example", "sessions": 10, "messages": 500 }],
  "diagnostics": []
}
```

This is preflight metadata only. It does not expose transcript content.

## Message records via list or search

`backscroll list --json --indexed-only` returns indexed items without full-text ranking. Records include:

- `schema_version`
- `source`, `source_path`, `project`, `uuid`
- `ordinal`, `timestamp`
- `role`, `content_type`
- bounded `text`

Results are ordered by `source_path`, `ordinal`, `timestamp`, and row id. Use filters such as `--project`, `--all-projects`, `--source`, `--source-path`, `--after`, `--before`, `--limit`, and `--indexed-only`.

Alternatively, `backscroll search "" --json --indexed-only` with an empty query string returns all indexed records (BM25 ranking disabled, deterministic ordering).

## Tool activity

`backscroll search "" --json --indexed-only --content-type tool` returns only messages with `content_type='tool'` — tool inputs (commands, args, file paths) and outputs/errors. Use this to audit what agents actually executed. Supports the same scope filters.

## Search is supplemental

`search` with a non-empty query string is a retrieval/UX surface. It may rank, truncate, aggregate, or use top-k semantics. Useful for investigation, but not sufficient for exhaustive audit discovery. Audit consumers should use `status --json --indexed-only`, `list --json --indexed-only`, and `search "" --json --indexed-only [--content-type tool]` as their corpus contract.

## Privacy and raw-content boundary

Backscroll stores normalized message text and bounded event snippets in SQLite. It does not make raw provider JSONL the downstream API contract, and examples avoid private absolute paths except where a user explicitly supplies one. Consumers that need full raw transcripts or unlimited tool output must implement an explicit opt-in path and their own redaction policy.
