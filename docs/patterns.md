# Pattern Discovery: `patterns` and `annotate`

Deterministic census aggregations over the indexed event store, plus the
write surface for agent-driven classification. Together they implement the
discovery loop designed in
`docs/superpowers/specs/2026-07-17-pattern-discovery-northstar-design.md`:
backscroll computes complete, reproducible censuses; an agent (or a human)
interprets them and writes labels back.

## Why not `search`?

`search` is retrieval: it answers "find what I can already name" with
top-N ranked snippets. Patterns live in aggregate frequency — no single
document contains them. Asking an LLM to "find patterns" over search
results makes it generalize from a handful of anecdotes. The `patterns`
command replaces that with censuses: counts, discovered templates,
correction candidates, and frequent sequences over the whole corpus.

## The five kinds

```bash
backscroll patterns --kind commands|failures|templates|corrections|sequences
```

Base flags for every kind: `--project` / `--all-projects`, `--tag`,
`--limit` / `--offset`, `--indexed-only`, `--json` / `--robot`.

### commands — what runs most

Top `(tool_name, command_head)` pairs by frequency. The absolute top is
usually unsurprising (Edit, Read); the informative reads are per-project
(`--project`) and per-tag (`--tag debugging`) stratifications.

### failures — what breaks

Failure signatures `(tool_name, is_error, exit_code)` for events with
`is_error = 1`. The census declares its coverage: events with no error
signal (`is_error` NULL) are excluded and counted. `exit_code=?` means no
exit code was recoverable (non-Bash tools have none; Bash text is
regex-mined and capped at 4000 runes by toolfmt).

### templates — recurring errors nobody named

Unsupervised Drain-style templates mined from error-bearing tool output
(`--min-support N`, default 3). Mining selects only rows with the
`error: ` output prefix or an `is_error = 1` event, and always excludes
tool-input serializations. Templates carry a `normalization_version`;
when extraction logic evolves the version bumps, so counts from different
mining epochs never silently mix.

### corrections — process-error candidates

Messages where the user likely corrected the assistant, from four
deterministic detectors (bilingual es/en lexicon 0.8, interrupt flag 0.5,
permission denial 0.4, Jaccard rephrase 0.6 — v1 priors pending the
calibration in `docs/eval/corrections-calibration.md`). Detectors run on
prose only (`role='user'`, content_type text/code) — tool output quoting
correction phrases does not fire them. Filter with `--min-confidence F`.

### sequences — frequent workflows (exploratory)

PrefixSpan over per-session category sequences (`inputs/categories.toml`,
versioned; embedded default wins over an older config copy).
`--min-support` (sessions containing the pattern), `--min-length`,
`--max-length` (hard cap, default 6 — non-contiguous subsequence mining
explodes combinatorially on repetitive sessions without it).
`--limit`/`--offset` paginate the MINED patterns; the input corpus is
never truncated.

## Trends: what grows beats what is common

```bash
backscroll patterns --kind failures --trend
```

Week-over-week bucketing (`--kind commands|failures` only; SQLite `%W`
weeks, Monday-based, not ISO 8601). A failure that is merely frequent is
often process-as-usual (tests failing during TDD); a failure that is
GROWING is a finding. Rows without timestamps are excluded and the count
is reported to stderr.

## The classification loop (`annotate`)

The resumable funnel that turns correction candidates into labeled data:

```bash
# 1. Agent fetches a batch of unlabeled candidates (uuid included in robot output)
backscroll patterns --kind corrections --pending --batch 50 --robot

# 2. Agent classifies each window and writes the label back
backscroll annotate --uuid <u> --kind correction --label "scope-exceeded"

# 3. Re-running step 1 automatically resumes: --pending is a LEFT JOIN
#    against annotations — already-labeled candidates disappear.
```

`annotate` resolves the uuid to canonical coordinates before writing and
rejects conflicting `--path`/`--ordinal` inputs; re-annotating the same
(message, kind) replaces the label. Labels are free-form until the
post-calibration `label_enum` freeze. Crash-safety comes from the query,
not from loop state: there is nothing to checkpoint.

## Operating notes

- Zero-result guidance goes to stderr; stdout stays clean for `--json`.
- A malformed `categories.toml` fails the command (non-zero exit) rather
  than masquerading as an empty result.
- Historical supply: rich capture exists for rows synced after migration
  v8; `rebuild` backfills expired files from stored text (lossy for tool
  events, marked `extraction_version=0`) and stale on-disk files re-parse
  at full fidelity during sync (capped per run, FIFO).
