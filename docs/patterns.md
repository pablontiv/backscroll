# Pattern Discovery: `patterns` and `annotate`

`--json` and `--robot` are mutually exclusive; combined flags fail before startup side effects. Empty robot results produce zero stdout bytes, with actionable hints on stderr.

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
`--limit` / `--offset`, `--json` / `--robot`. Corrections alone also accept
`--origin human|assistant|system|automation|unknown`.

### Eligibility and date bounds

`--tag` applies to every kind, including corrections and sequences. A missing
session tag yields zero candidates; partial tagging restricts support and share
to eligible sessions. Project and tag scope apply before aggregation, mining,
and final pagination, not to an already computed global result.

Satellite metadata resolves to the indexed event's proven UUID first, even
when retained coordinates differ. A UUID-less satellite can use path and
ordinal only when exactly one indexed item owns that coordinate. Historical
peers cannot multiply an event or lend it another project's scope. Tags and
timestamps come from the resolved item; sequence session grouping and ordering
use its canonical path and ordinal. Unscoped flat counts retain unresolved
satellites once, but they cannot prove project/tag/date eligibility. Trends
exclude those undated events and report the exclusion count.

Every API with temporal filters uses `[after, before)`: the lower bound is
inclusive (`>=`) and the upper bound exclusive (`<`); NULL timestamps cannot
satisfy date bounds. The patterns CLI advertises `--after` / `--before` only
for sequences, accepting ISO 8601 date strings. Internal command/failure and
template APIs also honor the same bounds; this does not add flags to other
pattern kinds. Sequence date filtering selects eligible events before mining.

### commands — what runs most

Missing command heads (persisted SQL `NULL` or empty strings) are grouped together
as `command_head: ""` in both flat and weekly output. This means unknown command
metadata, not an inferred command; persisted values are unchanged. Normalization
happens before grouping, ordering, and pagination, so there is only one unknown
group per tool (and week for trends).

Top `(tool_name, command_head)` pairs by frequency. The absolute top is
usually unsurprising (Edit, Read); the informative reads are per-project
(`--project`) and per-tag (`--tag debugging`) stratifications.

Pi native `toolCall` blocks contribute one event each, including tools other
than shell tools. `bash` arguments supply command heads before searchable input
is truncated. Native `toolResult` output is paired by `toolCallId` and contributes
no additional call event.

### failures — what breaks

Failure signatures `(tool_name, is_error, exit_code)` for events with
`is_error = 1`. The census declares its coverage: events with no error
signal (`is_error` NULL) are excluded and counted. `exit_code=?` means no
exit code was recoverable (non-Bash tools have none; Bash text is
regex-mined and capped at 4000 runes by toolfmt).

Pi uses only explicit native `isError` true/false as its result signal.
Interleaved and out-of-order results are matched by call ID. A missing result
or a result without `isError` remains unsignalled, even if its output looks
successful; explicit false counts as signalled success. Pi's native error flag
does not imply an exit code.

#### Failure census and weekly output compatibility (#147)

Failure JSON envelopes expose `total_events`, `signalled_events` (both
success and failure signals), and `unsignalled_events` (NULL signals), even
when no failures exist or pagination returns no signatures. Counts use the
same eligible project/tag/time population before pagination; total equals
signalled plus unsignalled. Signature `count` is failure incidence, NOT
signal coverage. The retained robot `result_N_coverage=count/signalled`
field is a signature's incidence among signalled events, not the proportion
of events carrying a signal.

With `--trend`, each week envelope carries its own census and each signature's
retained `signalled_events` denominator refers only to that week. The outer
census sums all dated eligible weeks. Weeks with zero failures (or signatures
removed by pagination) remain present with `patterns:[]`; signature pagination
and descending week/frequency ordering are unchanged. The exclusion diagnostic
counts ALL scoped events without timestamps, including successful and
unsignalled events, independently of date bounds.

This is an intentional output bug fix: command AND failure trend robot
payload keys are now `week_N_result_M_<field>`, retaining `week_N=<week>`
bucket markers and ordering. Consumers must stop relying on duplicate
`result_M_<field>` keys. Failure robot census keys are emitted once globally
and once per week as `week_N_total_events`, `week_N_signalled_events`, and
`week_N_unsignalled_events`; text output displays the same counts.
JSON census fields and zero-failure week envelopes are additive; historical
signature fields remain available. No schema or public flags change.

### templates — recurring errors nobody named

Unsupervised Drain-style templates mined from error-bearing tool output
(`--min-support N`, default 3). Sync uses structured `is_error` proof;
text-only backfill uses the `error: ` output prefix or a retained
`is_error = 1` event. Both exclude tool-input serializations and share
normalization and persistence, but backfill never invents structured proof.
Templates carry a `normalization_version`: observing the same signature
under current normalization advances an older row's epoch in either path,
without replacing its identity, matches, stored count, or first/last-seen
values. Repeated sync and backfill remain idempotent.

### corrections — process-error candidates

Messages where the user likely corrected the assistant, from four
deterministic detectors (bilingual es/en lexicon 0.8, interrupt flag 0.5,
permission denial 0.4, Jaccard rephrase 0.6 — v1 priors pending the
calibration in `docs/eval/corrections-calibration.md`). Detectors run on
prose only (`role='user'`, content_type text/code) — tool output quoting
correction phrases does not fire them. Filter with `--min-confidence F`.

For human-authored corrections, opt in explicitly:

```bash
backscroll patterns --kind corrections --origin human
```

`--origin` is valid only for corrections and accepts exactly `human`,
`assistant`, `system`, `automation`, or `unknown`. It filters the candidate
population before `--limit` / `--offset`, so pagination applies within the
selected origin. Origin is parser-backed provenance, not a guess from message
text or a historical stored `role`; rows without native parser evidence remain
`unknown`. The default is deliberately unchanged: without `--origin`, no
origin filter is applied and no origin field is added to output.

When the flag is present, text output adds `Origin: <value>`, robot output adds
`result_N_origin=<value>`, and each JSON candidate adds the case-sensitive
field `"Origin":"<value>"`. Those fields are omitted in all three formats
when the flag is absent.

### sequences — frequent workflows (exploratory)

PrefixSpan over per-session category sequences (`inputs/categories.toml`,
versioned; embedded default wins over an older config copy).
`--min-support` (sessions containing the pattern), `--min-length`,
`--max-length` (hard cap, default 6 — non-contiguous subsequence mining
explodes combinatorially on repetitive sessions without it).
`--limit`/`--offset` paginate the MINED patterns; the input corpus is
never truncated.

```bash
backscroll patterns --kind sequences --after 2026-01-01 --before 2027-01-01 --json
```

Sequence `--after`/`--before` select timestamped events before mining using
`[after, before)`: the lower bound is inclusive (`>=`), the upper exclusive (`<`).
Use ISO 8601 bounds; `YYYY-MM-DD` denotes the start of the UTC date. Unknown
timestamps cannot satisfy a bound. The example includes January 1, 2026 but
excludes January 1, 2027. These CLI date filters are for sequences only.

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
# 1. Agent fetches a batch of unlabeled candidates.
backscroll patterns --kind corrections --origin human --pending --batch 50 --robot

# 2. Inspect and label using a UUID only when its value is not empty or null.
if [ -n "${UUID:-}" ] && [ "$UUID" != "null" ]; then
  backscroll context --uuid "$UUID" --before 1 --after 1 --robot --max-tokens 2000
  backscroll annotate --uuid "$UUID" --kind correction --label "$LABEL"
else
  backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 1 --after 1 --robot --max-tokens 2000
  backscroll annotate --path "$SOURCE_PATH" --ordinal "$ORDINAL" --kind correction --label "$LABEL"
fi

# 3. Re-running step 1 automatically resumes: --pending is a LEFT JOIN
#    against annotations — already-labeled candidates disappear.
```

Before step 2, assign `UUID`, `SOURCE_PATH`, and `ORDINAL` from one selected machine-readable candidate and assign the intended free-form label to `LABEL`. A usable UUID is non-null and non-empty. The historical patterns contract can expose a legacy row as `"UUID":""` in JSON or `result_N_uuid=` in robot output; treat either form like null and use the exact published `source_path` plus `ordinal` from that same candidate. `context` is mandatory once candidate identity is known: ranked search is discovery, not an exact labeling window. It returns positional DB neighbors by stored ordinal, not a role-selected window and not raw provider records. If `context_ambiguous` is returned, do not label a guessed row; keep the diagnostic and disambiguate with a usable UUID when available. `annotate` resolves the UUID to canonical coordinates before writing and rejects conflicting `--path`/`--ordinal` inputs, including either selector supplied alone. An explicitly supplied `--ordinal` must be non-negative; omitting it with a UUID remains valid. Coordinate-only annotation uses the same exact absence and ambiguity boundary as `context`: multiple indexed rows at that coordinate are rejected before writing, even when they have distinct UUIDs. Use a usable opaque UUID to disambiguate; re-annotating the same (message, kind) replaces the label. Labels are free-form until the post-calibration `label_enum` freeze. Crash-safety comes from the query, not from loop state: there is nothing to checkpoint.

## Operating notes

- Ordinary snapshot-read, metadata-read, and mutation owners validate active manifests and attempt one incremental sync before executing. Busy readers may follow the last committed snapshot; remediation (`recover`) skips preparation/pre-sync and only syncs after installing a replacement. See [startup classes and exceptions](sync.md). Session, plan, and Markdown files are ingestion inputs; SQLite is the perennial record used by search, context, list, patterns, status, and validate. Use search for discovery. For one selected search result, use context with its published UUID when non-null, or its exact source path plus ordinal only when UUID is null.
- Zero-result guidance goes to stderr; stdout stays clean for `--json`.
- Robot mode on search emits `result_N_field=value` lines and escapes search string values with backslash as `\\`, carriage return as `\r`, and newline as `\n`.
- A malformed `categories.toml` fails the command (non-zero exit) rather
  than masquerading as an empty result.
- Historical supply: rich capture exists for rows synced after migration
  v8; `rebuild` backfills expired-file derived data from stored text (lossy for tool
  events, marked `extraction_version=0`) and stale on-disk files re-parse
  at full fidelity during sync (capped per run, FIFO). During startup reparse, V16
  origin provenance is enriched only when a configured source is still available
  to its reader; unavailable or expired sources remain `unknown`. Recovery can
  enrich an `unknown` duplicate from compatible proven evidence without the
  original source; contradictory proven origins are rejected.


## Invocation validation

Patterns accepts no positional operands. `--project` and `--all-projects`
are mutually exclusive; without either, the current-directory project is used.
Invalid operands, scope conflicts, and numeric values are rejected before
startup creates a database, acquires a lock, or syncs inputs.

Support, batch size, limit, offset, and sequence lengths must be nonnegative.
Confidence must be finite and in [0, 1], including both endpoints.
Zero sequence support/lengths preserve defaults (3 occurrences, lengths 2..6);
the effective minimum length must not exceed the effective maximum.
Zero batch remains inactive. Pagination is defined below.
These numeric checks apply even when a flag is irrelevant to the chosen kind.

### Pagination

`--limit` defaults to 20; explicit `--limit 0` is unlimited. `--offset`
skips final ordered candidates, including with an unlimited limit. This applies
uniformly to commands, failures (including trends), templates, corrections, and
sequences. Sequence mining always reads the full filtered corpus before paging
patterns. Equal-frequency candidates use logical identity tie-breaks (template
signature, command/failure identity, or sequence lexicographic order), not row IDs.
