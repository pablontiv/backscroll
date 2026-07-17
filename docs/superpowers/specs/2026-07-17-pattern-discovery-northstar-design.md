# North Star: Pattern Discovery

Date: 2026-07-17
Status: **APPROVED DESIGN** (survived two adversarial red-team rounds; see Provenance)
Scope: multi-slice north star. Each slice (F0a → F4) gets its own spec → plan → implementation cycle.

## Problem

Asking an agent to "find optimizable/correctable patterns in my sessions" via `backscroll search` has failed in every attempt. The cause is structural, not incidental: BM25/FTS5 is *retrieval* — it answers "find what I can already name" with top-N snippets. Patterns live in *aggregate frequency*: no individual document contains them. The agent ends up generalizing from 5 anecdotes instead of interpreting a census.

## Solution

Invert the responsibilities:

1. **backscroll computes deterministic censuses** — counts, discovered templates, correction candidates, frequent sequences — as reproducible SQL/Go computations.
2. **The agent only interprets complete tables** and writes classification labels back.

Discovery happens in two deterministic moments (template emergence at sync, frequency emergence at GROUP BY); the LLM's judgment is applied last, on candidates, never as the mining mechanism.

## Decided constraints

- Pure Go, no CGO, local-first, SQLite-centric (existing project constraints).
- **The agent IS the LLM**: backscroll never calls model APIs. It exposes candidates via CLI and accepts labels back via `annotate`. No keys, no network, no per-sync cost.
- **Perennity (user-mandated)**: the backscroll DB is the perennial event store. Session JSONL files expire (Claude Code cleanup ≈30 days); indexed sessions and agent annotations MUST survive their source files forever. The DB is *not* re-derivable from disk.
- **F5 (embeddings/semantic clustering) is out of scope** for this north star. Note: vector infra (chunks table, embedding BLOB, brute-force cosine `VectorSearch` in pure Go, `HybridSearch`, RRF) already exists from the M1 spike, deactivated. F5 re-enters only if F1–F4 leave the success criterion unmet.
- Fixed axes, discovered content: the *questions* are a small canned menu; the *answers* (templates, sequences, taxonomy) emerge from data. Free-form analysis remains possible later via a read-only `--sql` escape hatch (not in this north star).

## Architecture

```
SYNC (rich capture + extractors)      QUERY (new commands)             AGENT LOOP
readers extract uuid, tool_name,      backscroll patterns              skill reads census (--robot)
structured input, is_error,           --kind commands|failures|        interprets / classifies
was_interrupted (F0a)                 templates|corrections|           writes labels back
  → search_items (perennial,          sequences                         → backscroll annotate
     append-only upsert, F0b)         backscroll annotate              (anchored by uuid,
  → tool_events (perennial)           (write surface for labels)       batch + resumable)
  → message_templates
  → correction_signals
```

- **Satellite tables**: `search_items` keeps its schema and FTS triggers untouched. New tables reference stable message identity. Migrations v8+ — one clean migration per slice, per the repo's migration rule.
- **Perennial vs derived**: `search_items`, `tool_events`, and `annotations` are perennial (survive source expiry; only `purge` deletes them, and `purge` handles satellites explicitly). `message_templates` and FTS indexes are derived (rebuildable from the DB). Nothing is lifecycle-managed by CASCADE alone: cascade deletion of agent work is forbidden.
- **Identity**: message identity is `uuid` (extracted from JSONL as of F0a; globally UNIQUE in `search_items`, so a session file moved between project dirs upserts into the same rows instead of duplicating). Legacy rows (~220k, uuid NULL, sources mostly expired — no backfill possible) fall back to `(source_path, ordinal)`.

## Slices

### F0a — Rich capture at reader level (first; the perennity bet)

Everything not captured here is irrecoverable once the source JSONL expires.

- Extract per message, before serialization/cleaning: `uuid`, `tool_name`, structured tool input (enough to derive `command_head`), `is_error` as `*bool` (JSONL semantics: present=true/false, absent=NULL — Go's plain `bool` collapses NULL to false), `was_interrupted` (captured before `CleanContent` strips "Request interrupted" evidence).
- Applies to all three readers (Claude, Pi, OpenCode); each maps its own format.
- `models.Message` / `storage.IndexedMessage` gain the new optional fields; `search_items` rows get uuid populated from here on.
- Add `extraction_version` column: rows record which extractor version produced them. When extraction improves, old rows keep their version honestly instead of pretending uniformity; re-extraction happens only where source files still exist.

### F0b — Perennial sync semantics

- Session sync becomes **append-only upsert** keyed by uuid: existing rows untouched (ids stable forever), new rows appended. Wipe-and-reload (`DELETE FROM search_items WHERE source_path = ?`, today in `internal/storage/sync.go`) remains only for mutable sources (plans, external sources), where it is the correct semantics.
- `rebuild` redefined: re-derives FTS indexes from the DB via FTS5 external-content `INSERT INTO <fts>(<fts>) VALUES('rebuild')` and re-derives derived satellites; never re-reads disk as source of truth; never drops rows whose source file vanished.
- `purge --before` remains the only deletion path; it deletes satellite rows explicitly in the same transaction (no reliance on CASCADE for anything perennial).
- Ops stance: current scale (~220k items) leaves years of headroom; growth is linear in usage. **Known risk, accepted and documented**: a perennial DB holding irreplaceable annotations has no backup story yet — `backscroll export` is a named future slice, not part of this north star.

### F1 — Tool-event normalization + `patterns` command

- Migration adds `tool_events(item_uuid, tool_name, command_head, is_error, exit_code, mapping_version)`, anchored by message identity as defined in Architecture (uuid; legacy fallback `(source_path, ordinal)`). Populated at sync from F0a's structured capture (NOT parsed back out of toolfmt-serialized text, which is lossy). **Perennial** — with sources expiring, these rows are not re-derivable.
- `is_error` nullable; NULL means "no signal" and is never counted as success or failure. `exit_code` nullable, regex-mined from Bash output text only. `--kind failures` declares coverage ("N events without error signal excluded") and is meaningful mainly for CLI-style tools.
- Precedent note: this deliberately resurrects the shape of `session_events` (dropped in v2.0.0 as write-only dead weight). The difference that justifies it: it ships together with its consumer (`patterns`), never as a write-only table.
- `backscroll patterns` (ninth v2 command): canned, deterministic aggregations. Flag contract in two tiers — base flags valid for every kind (`--project`, `--all-projects`, `--limit`, `--offset`, `--json`, `--robot`), kind-specific flags on top (`--tag`, `--trend` for event kinds; `--min-support`, `--min-length` where mining applies). Early flag validation before DB open, per repo convention. Zero-result guidance to STDERR, clean STDOUT for `--json`.
- Signal/noise separation: aggregations stratified by `session_tags` (so TDD churn — "go test failed 200×" — doesn't drown signal) plus a week-over-week `--trend` dimension (what is *growing* matters more than what is *common*).

### F2 — Template mining (Drain-style, pure Go)

- Discovers message/error templates unsupervised (nobody writes them; they emerge from grouping messages differing only in variables).
- Line selection calibrated **per tool_name** against real outputs before launch (Go compiler errors lead, `go test` FAIL trails, panics sit mid-output — a global "first lines" rule fails).
- Only templates with `occurrence_count >= min_support` (default 3) persist — kills the singleton flood.
- Template identity: `(signature, normalization_version)` so historical counts survive normalization evolution. Derived table: rebuildable from perennial rows.
- Surface: `patterns --kind templates [--min-support N]`.

### F3 — Correction detection (process errors)

Process errors ("the LLM did X when asked Y") have no exit code; the signal is the user's reaction. Funnel: cheap detectors cast a wide net; the agent classifies only flagged candidates.

- Table `correction_signals(item ref, detector, confidence, window_ref)`.
- Detectors (all deterministic, pure Go): bilingual es+en correction lexicon (extends the `internal/tagging` regex approach, message-level); interrupt flags (from F0a); permission denials; rephrase-similarity between consecutive user messages via **Jaccard token overlap** (candidate generator, not judge — moderate precision is acceptable).
- **Mandatory calibration before any loop**: 50-candidate hand-labeled sample → measure per-detector precision → tune thresholds. Eval set lives in `docs/eval/` next to the existing recall eval. No calibration, no F3b.

### F3b — Classification loop (agent = LLM)

- `patterns --kind corrections` yields candidate windows (request + action + correction) with per-candidate progress state (`todo|done`); the agent classifies in batches of ~50; `backscroll annotate --uuid <u> --kind correction --label <l>` writes labels anchored by uuid. A run dying at 60% resumes where it left off — checkpointing is what makes the loop economically viable.
- Taxonomy: free-form labels first → **agent-side grouping** (the agent proposes a canonical grouping of observed labels + counts; the user approves) → frozen into a `label_enum` table (own migration; pre-freeze labels migrated in the same migration). Post-freeze, `annotate` rejects labels outside the enum. Extending the enum later = new migration. No embedding-based clustering — consistent with the agent-is-the-LLM constraint.
- `annotate` validates uuid existence and (post-freeze) enum membership before writing.

### F4 — Sequence mining (PrefixSpan, pure Go)

- Mines frequent tool-call sequences per session over **categories**, not raw commands: a versioned category map (`go test`/`go vet` → `GO_EXEC`; Read runs collapsed) shipped as TOML in `inputs/` (the repo's existing preset pattern), user-overridable, with `mapping_version` recorded so historical pattern counts stay interpretable.
- Flags: `--min-support` (default 3), `--min-length` (default 2). Documented as **exploratory** — output is candidate workflows for the agent/user to judge, not predictions.
- Surface: `patterns --kind sequences`.

## Dependencies

```
F0a → F0b → F1 → { F2, F3, F4 }
                     F3 → F3b
```

## Success criterion (operationalized)

Running the full loop on the real corpus produces **≥5 verified actionable findings**, where each finding:

- spans ≥2 sessions (or ≥5 tool events) — no singletons;
- includes a concrete mitigation proposal;
- is verified by the user (manual review).

"Top command: Read (50k)" does not qualify. If F1–F4 cannot meet this, that is the trigger to revisit F5 (embeddings), not to relax the criterion.

## Error handling, testing, compatibility

- Defensive parsing contract unchanged (skip malformed lines; skipped lines are never counted as data).
- Missing data is NULL, never invented; every census states its coverage.
- Testing: hermetic (`testEnv(t)`), synthetic JSONL fixtures per format; **explicit perennity tests** (sync → delete JSONL from disk → re-sync + rebuild → rows and annotations survive; growing session → stable ids); golden tests per aggregation (known table → exact text/robot/JSON output); F3 calibration eval in `docs/eval/`; `just ci` aggregate coverage ≥85% throughout.
- Migrations: new version per change, never touch old blocks. Existing 8 v2 commands unchanged; `patterns`/`annotate` are additive. F0b changes `rebuild`'s internal contract (no disk re-read) — documented in CLAUDE.md when shipped.

## Out of scope

F5 embeddings/clustering; `--sql` escape hatch; `backscroll export`/backup (named future slice); MATCH_RECOGNIZE (no engine support in SQLite/DuckDB — research transpiler only); any API calls from backscroll.

## Provenance

- Research: `docs/research/2026-07-pattern-detection-technologies.md` (supervised matching) and `docs/research/2026-07-pattern-discovery-technologies.md` (unsupervised discovery; corrected post-verification).
- Red team round 1 (3 lenses: data feasibility, architecture/lifecycle, product value): killed `exit_status` as designed (real JSONL only carries `is_error`), exposed interrupt-evidence destruction in `CleanContent`, annotation orphaning under delete+reinsert sync, GIGO in unstratified aggregations, and the unresumable classification loop.
- User-mandated perennity constraint reframed F0 (DB outlives sources).
- Red team round 2 (2 lenses: perennity/sync lifecycle, revised slices): found uuid extraction missing entirely (verified: 219,977/219,977 session rows NULL → F0a), tool_events not re-derivable (→ perennial), undefined similarity/clustering mechanisms (→ Jaccard / agent-side grouping), per-tool line-selection calibration, category-map versioning.
- Red-team claims audited and rejected: CASCADE as annotation fix (silent loss), English-only correction-density estimate, hash-desync crash scenario (refuted: rows+hash commit in one transaction), "REJECT F0" verdict (prerequisite failure, not concept failure).
