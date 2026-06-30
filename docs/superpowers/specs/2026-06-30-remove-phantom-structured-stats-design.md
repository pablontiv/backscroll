# Design — Remove the phantom structured-stats layer

**Date:** 2026-06-30
**Status:** Approved (brainstorming)
**Author:** pones + Claude

## Problem

`backscroll stats --group-by agent` crashed in v1.4.0 (`converting NULL to string
is unsupported`). The crash was fixed (commit `60e38cd`), but verifying the fix
surfaced a deeper issue: the entire structured-stats surface is a **phantom** —
the query/CLI layer exists, but nothing ever populates the data it reads.

### Root of the phantom

The `session_events` table has structured columns (`actor`, `tool_name`,
`tool_id`, `command`, `cwd`, `exit_code`, `is_error`, `event_type`). The CLI
(`stats --type/--tool/--group-by`, `list --type/--tool/--command`) reads them.
But the **population layer never existed**:

- `sync.go` inserts only `event_type='message'` rows with `role` + `snippet`.
- The `models.Message` type has no `tool_name`/`actor` fields; readers flatten
  tool data into `Content` text (`content_type='tool'`) — for search, not stats.
- No reader emits structured `tool_call` events.

The v2 SDD (`backscroll-cli-v2-input-native-cli`) planned the query surface under
the explicit assumption that structured data was *"already available in
session_events"*. That assumption was false. There was no decision to build the
population layer, and no decision to descope it — it fell through an unverified gap.

### Demand analysis (live corpus, 184,927 events, 3 projects)

| `--group-by` dimension | Benefit | Evidence |
|---|---|---|
| `tool`  | Marginal | 151,256 tool events exist, `tool_name` is extractable from text, but `search --content-type tool` already finds them; stats would only add counts. |
| `agent` | **None** | 0 subagent sessions in the corpus; "agent" identity is not stored. |
| `project` | **None** | 99% of tool content is project `unknown` (149,225 / 151,256). |

No session in any project benefits from the structured-stats surface as built.
`search --content-type tool` (backed by `search_items` + the `tool_fts` trigram
index) already serves every "what tool ran / what failed" need.

## Decision

**Remove the entire phantom layer** and redirect users to
`search --content-type tool`. Two confirmed choices:

1. **Remove the `stats` command entirely** — it is 100% built on the phantom;
   every output is `<unknown>` or `message:N`. A gutted stats that only counts
   messages would mislead more than it helps.
2. **Drop the `session_events` table** via a new migration (V5). "Kill the whole
   layer," not leave a dead table written on every sync.

## Scope

### Removed (code)

- `cmd/backscroll/stats.go` — the whole `stats` command, `groupEvents`, `StatEntry`.
- `cmd/backscroll/list.go` — the structured-filter path (`--type/--tool/--command`
  → `ListSessionEventsV2`). The normal `list` path (over `search_items` via
  `ListSessions`) stays.
- `internal/storage/queries.go` — `ListSessionEventsV2`, `StructuredEventRow`.
  (The v1.4.0 NULL-scan fix is removed along with the function — removing the
  code removes the bug.)
- `internal/storage/records.go` — `QuerySessionEvents`, `SessionEvent`,
  `SessionEventQuery` (already dead: test-only callers).
- `internal/storage/sync.go` — the `INSERT INTO session_events` loop.
- `internal/storage/queries.go` purge path — drop the `DELETE FROM session_events`.
- Tests covering the above (including the regression tests added with the crash fix).

### Removed (schema, migration V5)

- New `applyV5Migration`: `DROP TABLE IF EXISTS session_events;` plus its indexes,
  and a `schema_versions` row `(5, 'V5 drop phantom session_events', ...)`.
- Follow the schema rule: **new migration block only**; never edit V1–V4.

### Kept (unchanged)

- `search` incl. `--content-type tool` (`search_items` + `tool_fts`).
- `list`, `read`, `status`, `validate`, `rebuild`, `purge`, `config`.
- `tool_fts` and the split-FTS work — independent of `session_events`.

### Docs / skill

- `CLAUDE.md` — Module/Package Layout (no package is removed, but command list,
  the `stats` line, and the content-type/structured-events design notes change),
  `session_events`/`ListSessionEventsV2`/`QuerySessionEvents` references.
- `README.md` — remove the `stats` section and the `stats` row in the command
  table; point tool-activity use cases to `search --content-type tool`.
- Backscroll skill (`.claude/skills/backscroll/SKILL.md`) — remove the `stats`
  rows from the command table and the invocation mapping; the subagent-stats
  workflow (5.2) becomes a `search --content-type tool` example.
- `docs/*.md` mentioning `stats`/structured events.

## Data-flow impact

Before: `JSONL → readers → Message → sync → search_items (FTS) + session_events (phantom)`.
After: `JSONL → readers → Message → sync → search_items (FTS)`. The
`session_events` branch is gone; nothing else in the pipeline changes.

## Migration / compatibility

- `DROP TABLE session_events` on existing DBs is safe: the table was write-only
  dead weight; `search_items` (the queryable index) is untouched. `rebuild`
  continues to repopulate `search_items`.
- CLI contract change: `stats` is removed. This is a deliberate breaking change
  to the v2 surface, justified because the command never produced useful output.

## Testing

- Delete tests bound to removed code (stats, `ListSessionEventsV2`,
  `QuerySessionEvents`, structured `list` filters).
- Add/adjust: `list` without structured flags still works; `purge` no longer
  references `session_events`; migration runner reaches V5 and `session_events`
  no longer exists (`PRAGMA table_info` empty / query errors as expected).
- Gates: `just check`, `just test`, `just coverage-check` (≥85% per package) must
  pass. Removing code tends to raise coverage; watch for any package that drops
  below floor once its tested-but-removed lines are gone.

## Risks

- **Coverage floor:** deleting heavily-tested storage functions could shift a
  package's ratio; verify `just coverage-check` before push.
- **Hidden consumers:** confirm no other command imports the removed storage
  functions before deleting (grep done: only `stats`/`list` structured path).
- **Docs drift:** the pre-push hook blocks Go-source changes without doc updates;
  CLAUDE.md/README/skill must be updated in the same change.
