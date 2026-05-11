# Downstream audit integration contract

Backscroll is the corpus and structured-query layer. Downstream tools such as a session-audit runner own deterministic findings, thresholds, redaction policy, report rendering, and any ADR/KE/backlog creation.

## Deterministic audit flow

Use an explicit snapshot boundary before auditing:

```bash
# 1. Maintainer-controlled index refresh, outside the audit read phase
backscroll inputs validate
backscroll sync

# 2. Audit preflight against the existing SQLite snapshot
backscroll status --json --indexed-only

# 3. Scope discovery from indexed session metadata
backscroll sessions query --jsonl --indexed-only --all-projects --limit 0

# 4. Complete normalized event stream for deterministic consumers
backscroll events query --jsonl --indexed-only --all-projects --limit 0
```

`--indexed-only` opens the existing SQLite index read-only and does not run input discovery or sync. Use it for repeatable audit reads. Commands that require an existing index fail or report `index.usable = false` instead of silently creating a new corpus boundary.

## Status JSON

`backscroll status --json` emits one versioned JSON document:

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

## Session detail JSONL

`backscroll sessions query --jsonl` streams indexed message records without a search term. Each line includes:

- `schema_version`
- `source`, `source_path`, `project`, `uuid`
- `ordinal`, `timestamp`
- `role`, `content_type`
- bounded `text`

Records are ordered by `source_path`, `ordinal`, `timestamp`, and row id. Use filters such as `--project`, `--all-projects`, `--source`, `--source-path`, `--after`, `--before`, `--limit`, and `--indexed-only`.

## Event JSONL

`backscroll events query --jsonl` streams normalized session events. Each line includes:

- `schema_version`
- `source`, `source_path`, `project`
- `ordinal`, `timestamp`, `event_type`
- `actor`, `role`
- event-specific metadata: `tool_name`, `tool_id`, `command`, `cwd`, `exit_code`, `is_error`
- bounded `snippet`

Supported event types include `message`, `tool_call`, `tool_result`, `command`, `error`, `metadata`, and `other`. Use `--event-type` plus the session scope filters for deterministic buckets.

## Search is supplemental

`search`, `topics`, and `insights` are retrieval/UX surfaces. They may rank, truncate, aggregate, or use top-k semantics. They are useful for investigation, but they are not sufficient for exhaustive audit discovery. Audit consumers should use `status --json --indexed-only`, `sessions query --jsonl --indexed-only`, and `events query --jsonl --indexed-only` as their corpus contract.

## Privacy and raw-content boundary

Backscroll stores normalized message text and bounded event snippets in SQLite. It does not make raw provider JSONL the downstream API contract, and examples avoid private absolute paths except where a user explicitly supplies one. Consumers that need full raw transcripts or unlimited tool output must implement an explicit opt-in path and their own redaction policy.
