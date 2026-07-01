# Diagnostic UX & doctor hardening — design

## Overview

A cluster of low-risk quick wins surfaced by running `backscroll-doctor` (the self-diagnostic skill) against backscroll's own indexed history. All items are cheap, carry no design risk, and are grouped to ship together in one change.

Two fronts:

1. **Query UX** — `search`/`list` fail silently today. Zero results return an empty payload with no guidance, and tool queries whose term is under 3 characters match nothing because of the `tool_fts` trigram tokenizer, again with no signal. Both leave the user guessing.
2. **Doctor hardening** — now that the skill lives in the repo, its query catalog and diagnostic sessions get indexed, producing self-referential noise; Pi turn telemetry (`turn_end`/`turn_start`) is also unfiltered and yields false positives (`unknown flag`, `unknown command`).

**Out of scope** (noted in the audit, deliberately excluded from this change): O18 project=unknown bucketing, O09 embeddings/semantic search wiring, the unused `source_metadata` column, and the `session_dir`/`session_dirs` naming inconsistency.

## Motivation & evidence

From the self-audit (index sample: 1702 files / ~186k messages):

- Zero-result friction was the single most frequent pattern; users have no recovery guidance and often just needed `--all-projects` or `--content-type tool`.
- The trigram floor is documented in `CLAUDE.md` ("queries shorter than 3 characters will match zero results") but nothing warns at runtime.
- The doctor validation run produced 6 false-positive error signatures — `turn_end id=unknown:…` telemetry and snippets quoting the doctor's own query catalog — all caught by the mandatory verify step but avoidable at the input.

## Components

### C1 — Zero-result diagnostics (`cmd/backscroll/search.go`, `list.go`)

When a command yields 0 rows, print a short suggestion block to **STDERR** (never STDOUT, so `--json` stays a clean, parseable empty payload).

- Suggestions are conditional: if the query was project-scoped (no `--all-projects`), suggest `--all-projects`; always suggest `backscroll status`; if the query looks like a path/command, mention `--content-type tool`.
- Exit code stays 0; STDOUT structure unchanged.

**Acceptance**
- A 0-result `search`/`list` prints ≥1 actionable suggestion to STDERR.
- STDOUT is byte-identical to today in `--json` (empty structure).
- Tests cover: project-scoped query suggests `--all-projects`; a query already using `--all-projects` does not.

### C2 — Short tool-query warning (`cmd/backscroll/search.go`)

When the effective search term is under 3 characters and the query targets tool content (trigram), print a STDERR warning that the trigram tokenizer needs ≥3 characters and results may be empty. The query still runs.

- Warning only; no error, no exit-code change, no STDOUT change.

**Acceptance**
- `search "go" --content-type tool` warns on STDERR.
- A ≥3-character term does not warn.
- STDOUT unchanged.

### C3 — `gather.sh` noise hardening (`.claude/skills/backscroll-doctor/assets/gather.sh`, `SKILL.md`)

Two input-side filters plus a doc clarification:

1. Extend the `NOISE` regex to drop Pi turn telemetry (`turn_end`, `turn_start`, `turn-end`, `turn-start`) — the source of the `unknown flag`/`unknown command` false positives.
2. Filter self-referential lines that merely quote the doctor's own query catalog (e.g. a snippet containing several bracketed query names in sequence).
3. `SKILL.md`: reaffirm that tool-error signature hits are **leads**, not facts — the mandatory verify step remains the primary guard; this change only lowers input noise.

**Acceptance**
- The errors angle no longer surfaces `turn_end id=unknown` lines.
- A snippet that only echoes the query catalog is dropped.
- `SKILL.md` states tool-error hits are unverified leads.

## Testing

- C1/C2: table-driven tests in `cmd/backscroll` asserting STDERR content and unchanged STDOUT across scoped/`--all-projects` and short/long-term branches. Use the existing `run(stdout, stderr, args)` harness so stderr is captured separately.
- C3: `gather.sh` is shell; validate by running it against the live index and asserting the known false-positive signatures no longer appear (manual/CI smoke, not a Go test).
- `just check fmt test` green; per-package coverage ≥85% maintained.

## Delivery

Single change; estimated well under the 400-line budget. No schema migration, no new package, no dependency change. `cmd/backscroll` docs (CLAUDE.md command descriptions) get a one-line note about zero-result hints to satisfy the docs-update gate.
