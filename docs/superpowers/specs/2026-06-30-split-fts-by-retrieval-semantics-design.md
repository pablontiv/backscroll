# Split FTS Index by Retrieval Semantics — Design

**Date:** 2026-06-30
**Status:** Approved (design phase)

## Problem

After the reader-per-agent feature (merged 2026-06-30) made tool inputs/outputs
searchable, **everything lives in one FTS5 index** — `messages_fts` over
`search_items`, tokenizer `porter unicode61` — discriminated only by a
`content_type` column (`text` / `code` / `tool`). Tool content now dominates the
corpus (~82% of ~184K items), and two distinct retrieval semantics are crammed
into one table with one tokenizer and one BM25 IDF.

Two failures were measured against the live index on 2026-06-30:

1. **Tokenizer broken for tool content.** Searching the exact path
   `internal/storage/sync.go` ranked that file **last** of 5 hits (BM25 −9.09);
   `hybrid.go` won (−3.49). Porter shreds the path into stemmed fragments
   (`intern` / `storag` / `sync` / `go`), so exact path/command/error lookup —
   the single most common tool query — does not work.
2. **Prose crowding.** Top-30 BM25 for "architecture decision" returned 30 tool /
   0 text; "error handling" returned 29/1. Conversational/decision search is
   buried under tool output. (Honest nuance: "migration plan" and "tradeoff
   approach" were roughly proportional to the 82% baseline — so this is partly
   corpus dominance, not always *extra* crowding.)

The `content_type` column already supports filtering (`--content-type tool`), so
the value of this work is **not** filtering. It is (a) a **per-table tokenizer**
— FTS5 tokenizers are fixed per table, so path/command-aware tokenization for
tools requires a separate index — and (b) a **per-table BM25 IDF**, because a
mixed corpus skews IDF for multi-term prose queries.

## Utility audit (why this is worth doing)

Before designing, real-world utility was validated empirically. ~65 actual
`backscroll` invocations across all projects / all time (where the tool was run
to retrieve something) were audited by three parallel agents, clustered as
harness-tooling / infra-ops / apps+opencode. Verdict on "would tool-call data
have helped this search?":

| Verdict | Share |
|---|---|
| TOOL-WOULD-HELP | ~51% |
| ALREADY-TOOL-QUERY | ~11% |
| PROSE-WAS-RIGHT | ~35% |
| UNCLEAR | ~3% |

Combined, **~62% of real searches targeted execution data, not prose.** Honest
caveat: the infra cluster used a looser/larger count and skews the aggregate;
weighting the three clusters equally drops TOOL-WOULD-HELP to ~42%. Either way,
tool queries are real and frequent.

Clear domain pattern: **infra/ops work is "command archaeology"** — "what exact
command flashed the USB?", "what error did the Harvester installer throw?", "what
did the sensor read at throttle?". Those answers live in stdout/stderr and the
commands themselves, not in prose. Decision/architecture work is prose-answerable.

Conclusion: indexing tool calls was the right call (not speculative), and the
split is justified on **both** sides — tool queries dominate, and mixing them
into the prose table demonstrably degrades both. This reorders priority: the #1
real pain is exact path/command/error lookup, which the porter tokenizer breaks
today, so the tool-activity index is the highest-leverage slice.

## Goal

Split the single mixed FTS5 index into separate indexes by **retrieval
semantics**, not by raw type label — delivered in two independently shippable
slices:

- **Slice 1 (now):** a dedicated tool-activity index with a path/command-aware
  tokenizer. Fixes exact lookup *and*, as a side effect of moving tool rows out,
  fixes prose crowding.
- **Slice 2 (future):** a dedicated reasoning index, gated on a prerequisite
  decision to stop dropping `thinking` blocks.

## Why this slicing

FTS5's tokenizer is **fixed per table**. You cannot give the `tool` rows a
different tokenizer than the `text` rows inside `messages_fts`. Therefore "fix
the tool tokenizer" *necessarily* means "create a second FTS5 table" — which is
exactly the first brick of the full split. Slice 1 is the foundation of the
design, not throwaway work.

The only piece genuinely deferrable is the reasoning index, because it depends on
a separate prerequisite: today `thinking` blocks are **dropped** at parse time by
ClaudeReader/PiReader. Indexing them is a privacy + volume decision, not free, so
it does not block Slice 1.

---

## Slice 1 — Tool-activity index (now)

### Approach

1. **New FTS5 table `tool_fts`** with a path/command-aware tokenizer (see
   Decision 1), plus its `fts5vocab` companion to match the `messages_fts`
   pattern.
2. **Move `content_type='tool'` rows out of `messages_fts` into `tool_fts`.**
   After the move, `messages_fts` holds prose semantics only (`text` + `code`,
   porter). This is what fixes crowding — the 82% tool mass leaves the prose
   table.
3. **New migration version 4.** The current max schema version is 3 (`V3
   embedding blob column`). Per the CLAUDE.md schema-migration rule, add a new
   `version = 4` block in `setupSchema()`; never edit an existing migration
   block. The migration creates `tool_fts` + vocab, rewrites the
   trigger set so tool rows route to `tool_fts` and text/code rows route to
   `messages_fts`, and triggers a **full rebuild** so existing rows land in the
   correct table under the correct tokenizer.
4. **Query routing in `search.go`:**
   - `--content-type tool` → query `tool_fts` only.
   - `--content-type text|code` (and the prose default) → query `messages_fts`
     only.
   - No content-type filter ("search everything") → query both and merge (see
     Decision 2).

### Trigger redesign

Today the triggers (`migrations.go:245-254`) insert every row's `text` into
`messages_fts` unconditionally. The new triggers must branch on
`new.content_type`:

- `content_type='tool'` → insert into `tool_fts`.
- otherwise → insert into `messages_fts`.

Delete/update triggers mirror the same branch so a row is removed from whichever
index it lives in. Because `content_type` is immutable for a given row in
practice (set at sync time), an update that changed it would need delete-then-
insert across tables; the migration should assume content_type is stable and the
sync path should never mutate it.

### What Slice 1 fixes

- **Exact lookup** (tokenizer): `internal/storage/sync.go`, `go test ./...`,
  `exit code 1` become findable with correct ranking.
- **Prose crowding** (side effect of the move): "architecture decision" stops
  returning 30 tool hits because tool rows are no longer in `messages_fts`.

---

## Slice 2 — Reasoning index (future)

### Prerequisite (separate decision)

`thinking` blocks are currently dropped by ClaudeReader/PiReader. Indexing them
is a privacy + volume decision that must be made before this slice. This design
records the dependency but does **not** decide it.

### Approach (once unblocked)

1. Stop dropping `thinking` blocks in the relevant readers; classify them with a
   new `content_type='reasoning'` (or equivalent), opt-in via input/config so a
   user can choose not to index reasoning.
2. **New FTS5 table `reasoning_fts`** with a prose-like tokenizer (porter), plus
   vocab companion. New migration version; reindex reasoning rows.
3. Extend query routing: a `--content-type reasoning` selector and inclusion in
   the "search everything" merge.

---

## Open decisions (resolved here, revisitable in plan/review)

### Decision 1 — Tool tokenizer: **trigram** (recommended)

- **trigram** (chosen): true substring/exact matching — finds `torage` inside
  `storage`, matches partial paths and command fragments. Cost: larger index;
  FTS5 trigram requires queries of ≥3 characters and changes match semantics
  (substring, not token). Best fit for the measured command-archaeology pattern.
- *Alternative — `unicode61` without porter stemming:* lighter index, keeps
  whole tokens (`internal`, `storage`, `sync`, `go`) without shredding them via
  stemming, but no substring matching. Fixes the *stemming* failure but not
  partial-fragment lookup.

Recommendation: **trigram**, because the dominant real query is exact/partial
path/command/error lookup, which substring matching serves directly.

### Decision 2 — Cross-type "search everything" merge

With tool and prose in separate tables, a query without `--content-type` must
merge results from both. BM25 scores are **not comparable across tables**
(different corpora, different IDF). Approach:

- Run both queries, take top-N from each, and merge by a normalized rank rather
  than raw BM25 (e.g., min-max normalize each table's scores, or interleave by
  per-table rank). Document the limitation: cross-index ordering is approximate.
- This only affects the no-filter path; `--content-type`-scoped queries stay
  exact and fully ranked.

## Affected code

| Area | File(s) | Change |
|---|---|---|
| Schema/migrations | `internal/storage/migrations.go` | New migration version: `tool_fts` + vocab, branched triggers, rebuild trigger |
| Sync/indexing | `internal/storage/sync.go` | Ensure inserts honor content_type routing (or rely on triggers) |
| Search routing | `internal/storage/search.go` | Route by `--content-type`; cross-table merge for no-filter |
| Rebuild | `internal/storage/queries.go` | `optimize` and integrity checks must cover both FTS tables |
| Docs | `CLAUDE.md` | Document `tool_fts` index, tokenizer, and the two-index query model |

## Testing

- **Migration:** an existing v3 DB upgrades cleanly; tool rows end up in
  `tool_fts`, text/code in `messages_fts`; no rows lost (count before == count
  after across both tables).
- **Tokenizer regression:** the measured failure case — searching
  `internal/storage/sync.go` — now ranks that file first, not last.
- **Crowding regression:** "architecture decision" against `messages_fts` returns
  prose, not 30 tool hits.
- **Routing:** `--content-type tool` hits only `tool_fts`; prose default hits
  only `messages_fts`; no-filter merges both.
- **Coverage:** keep per-package floor ≥85% (pkcov gate); add storage unit tests
  for the new migration block and routing branches.

## Risks / caveats

- **Full rebuild required** — the migration reindexes the whole corpus; on a
  513MB DB this is a one-time cost the migration must handle without data loss.
- **Cross-index BM25 is not comparable** — the no-filter "search everything" path
  is approximate by construction (Decision 2).
- **Trigram index size** — tool content is the bulk of the corpus; a trigram
  index over it will be larger than the equivalent porter index. Acceptable given
  the move keeps total index growth bounded (rows move, not duplicate).
- **Slice 2 is gated** — reasoning indexing must not be started until the
  thinking-block privacy/volume decision is made.

## Precedent

`session_events` is already a separate structured store for tool-call metadata,
so "separate store by purpose" is an established pattern in this repo.
