# Backscroll — North Star and Milestone Map (Master Design)

Date: 2026-07-02
Status: Approved design, pending implementation plans

This is the master spec: it fixes the north star, maps all milestones toward it
(M1–M4), and details M1 in full. M2–M4 are scoped at milestone level and get
their own detailed designs when their turn comes. Implementation plans are
derived per slice from this document.

## Milestone Map

| Milestone | Theme | Success signal |
|-----------|-------|----------------|
| M1 — Episodic Recall v1 | Automatic recall + complete corpus | Agents recover prior work unprompted; eval-set recall@5 measured |
| M2 — Retrieval Quality by Data | Execute the spike verdict; permanent benchmark | Ranking decision (hybrid vs lexical) documented with numbers; ~50-query benchmark in repo |
| M3 — Downstream Consumers | O04 event query API + read-only programmatic surface | Pinata/poness consume backscroll via stable JSON contract |
| M4 — Full Episodic Ecosystem | More readers, retention policies, engram handoff | backscroll feeds engram distillation; new agents onboard via reader-per-agent |

Sequencing is strict M1 → M2 → M3 → M4: M2 depends on M1's spike and eval-set;
M3 only makes sense once M1 proves recall works; M4 builds on a proven,
consumed index.

## North Star

Backscroll is the definitive local episodic memory for coding agents: everything
that happened — commands, errors, decisions, reasoning — exactly recoverable, and
consulted automatically before agents start work.

It complements engram, it does not compete with it: backscroll remembers *what
happened* (episodic layer); engram distills *what we learned* (semantic and
operational layer). This resolves the identity question raised by the wiki entity
that marks backscroll as a "predecessor that failed in practice": the failure mode
was adoption, not design. The 2026 state of the art validates the architecture
(local-first, exact/agentic search over vector RAG for code; dual FTS split by
retrieval semantics). This milestone attacks adoption directly while completing
the corpus.

## Decisions Locked Before This Design

1. **Identity**: episodic layer complementing engram. Not a full memory system,
   not downstream-consumer infrastructure (O04/Pinata is out of scope).
2. **Success criteria**: automatic recall by agents + complete and correct corpus
   + a minimal dose of retrieval quality (no embeddings activation by default).
3. **Recall mechanism**: reinforced `/backscroll` skill + native CLI. MCP was
   rejected on verified evidence: MCP costs 4-32x more tokens than CLI for
   identical operations (Scalekit benchmark, 75 head-to-head runs), each tool
   schema injects 550-1,400 tokens, and reliability degrades to 72% on complex
   tasks; Anthropic's own "code execution with MCP" post validates CLI-style
   access (98.7% context reduction).
4. **Delivery**: no pull requests. Direct commits to `main`, push after each
   slice. Each push triggers the existing automated release flow (CI computes
   semver from conventional commits, builds, publishes), so every slice ships to
   production on its own.

## M1 — Episodic Recall v1 (Detailed Design)

Three tracks in a single arc. Dependency order: A1 (O18) first; then Track A
and Track B proceed in parallel; Track C closes the milestone using the
eval-set as evidence.

### Track A — Automatic Recall

**A1. O18 — workspace bucketing by cwd** (precondition for everything)

Plumb the session `cwd` through the pipeline: reader → `ParsedFile` → sync →
`projects.Identify()`. Today sessions never reach `Identify()` with a cwd and
resolve to `unknown`, which breaks project-scoped recall. Must handle the known
cross-host path gotcha (`/home/shared` vs `/Users/Shared` roots on a synced
index): resolution maps equivalent roots through the project registry, not raw
string matching.

**A2. Recall-first `/backscroll` skill**

Redesign of the existing skill with three concrete changes:

- **Aggressive triggers**: not only explicit recall phrases ("prior sessions",
  "we already did this") but also the start of work on a feature or bug that may
  have history — beginning work triggers a prior-work lookup.
- **Agent output contract**: the skill consumes `search --robot --fields`
  (minimal payload, no prose) with a fixed recipe: project-scoped first, fall
  back to `--all-projects`, and use `--content-type tool` for execution-shaped
  queries (commands, paths, errors).
- **Documented token budget**: the skill declares how much context a lookup may
  spend, using the existing `--max-tokens` flag.

**A3. Eval-set of real queries**

Approximately 20 queries mined from real indexed sessions (of the shape "where
did I run X", "what error did Y give", "what did we decide about Z"), each with
an annotated expected result. Lives in the repo (`docs/eval/`), runs as an
optional integration test. This is the yardstick for the whole milestone: Tracks
B and C are measured against it.

**Error handling**: if the index is stale or the database is locked, the skill
degrades with a clear message instead of silence (the shipped zero-result stderr
hints already help here).

### Track B — Complete Corpus

**B1. Slice 3 — OpenCode tool parts** (cheapest, ships first)

Extend `OpenCodeReader` to capture `tool` parts (`state.input` + `state.output`),
serialized with the existing `toolfmt` serializer from Slice 1. No new reader, no
migration: rows enter `tool_fts` as `content_type='tool'`. Low risk.

**B2. Slice 2 — Pi reasoning** (requires explicit privacy decision)

Index Pi's real reasoning text. Claude stays out: the API redacts thinking
blocks, there is no data. Two decisions fixed at design time:

- **Privacy**: reasoning is opt-in per input manifest (`index_reasoning = true`
  in `pi.inputs.toml`), default off — consistent with the declarative input
  philosophy.
- **Destination**: `messages_fts` (prose, porter tokenizer) with a new
  `content_type='reasoning'`, filterable via `--content-type reasoning`.
  Requires migration v7 (new version block; existing migration blocks are never
  modified, per repo rule).

**B3. Slice 4 — Retire the declarative input engine** (closing cleanup)

Delete `JsonlReader` + `ParseDeclarative`; consolidate on reader-per-agent
(ClaudeReader, PiReader, OpenCodeReader). Pure deletion, zero behavior change —
but it goes last: if B1/B2 reveal any input still depending on the declarative
path, we learn it before deleting. Operational reminder: when deleting a
package, update the Module Layout and Package Layout sections in CLAUDE.md or
the pre-push hook rejects the push.

**Track testing**: each slice ships with fixtures from real (anonymized)
sessions, and the eval-set gains 3-5 queries answerable only with the new
content (reasoning / OpenCode tools) — new corpus proves its value, not just its
existence.

### Track C — Minimal Retrieval Quality

**C1. RRF merge across indexes**

Unfiltered searches currently merge `tool_fts` and `messages_fts` via min-max
BM25 normalization, which is admittedly approximate across different tokenizers.
Replace with Reciprocal Rank Fusion: rank-based, immune to incomparable score
scales. Small, local change (the unfiltered branch of `Search()` only),
measured before/after with the eval-set. The dormant O10 RRF code is partially
revived for the one purpose it serves today.

**C2. Eval-set as regression benchmark**

The Track A eval-set runs before each slice push (local gate, not a required CI
gate, to keep the release flow unblocked). Metric: recall@5 over the ~25
queries. A change that lowers recall is visible before it ships.

**C3. Embeddings spike** (time-boxed, end of milestone)

Experimental branch: activate the dormant O09/O10 code (ONNX provider + hybrid
RRF), run the eval-set BM25-only vs hybrid, measure recall@5, latency, and setup
weight. Output is a documented decision with numbers: **activate / defer /
delete** the dormant code. Either way the phantom debt is settled. Constraint:
if activation would break pure-Go/no-CGO, it is an automatic no-go; the pure-Go
`sqlite-vec` path is the alternative to evaluate inside the same spike. The
spike merges a decision report (docs + engram), not necessarily code.

### M1 Delivery

- Direct commits to `main` with conventional commits; push after each completed
  slice. CI releases automatically on every push.
- Slice order: A1 (O18) → A2+A3 (skill + eval-set) → B1 (OpenCode) → B2 (Pi
  reasoning, migration v7) → C1 (RRF) → B3 (retire declarative) → C3 (spike).
  B1 may proceed in parallel with A2/A3.
- Gates per slice: `just check`, `just test`, coverage ≥85% (pre-push enforced),
  eval-set recall@5 from A3 onward.

### M1 Success Criterion

An agent in a fresh session of any indexed project recovers relevant prior work
without manual invocation, and the eval-set passes with measurable recall.

## M2 — Retrieval Quality by Data

Execute the verdict of M1's embeddings spike instead of debating it:

- If the spike shows hybrid retrieval beats BM25 on the eval-set at acceptable
  latency and without breaking pure-Go/no-CGO: activate it (ONNX provider or
  pure-Go sqlite-vec, whichever the spike favored) behind a config flag.
- If not: delete the dormant O09/O10 code paths and stay lexical, closing the
  phantom debt permanently.
- Grow the eval-set into a permanent benchmark (~50 queries, covering tool,
  prose, reasoning, and cross-project shapes) that runs as the standing
  regression gate.
- Ranking tuning informed by the benchmark: per-content-type weights and a
  recency boost are the two candidates surfaced by usage mining.

Success signal: the ranking decision is documented with numbers, and the
benchmark lives in the repo as the standing quality gate.

## M3 — Downstream Consumers

What M1 deliberately excluded returns as its own milestone, once recall is
proven:

- O04 event query API: deterministic, ordered session-event queries with a
  stable JSON contract (the design exists; implementation tasks T006+ were
  never built).
- `OpenReadOnly()` promoted to an official programmatic surface with documented
  guarantees for external consumers (Pinata, poness).
- Contract tests so downstream consumers can pin against a versioned schema.

Success signal: at least one real consumer (Pinata or poness) reads backscroll
through the contract in production use.

## M4 — Full Episodic Ecosystem

- New readers via the reader-per-agent pattern for additional agents/formats as
  they appear in the toolchain.
- Retention and archival policies: purge by policy (age, project, content type,
  size budgets), not only by date.
- Formal backscroll→engram handoff: backscroll as the episodic source engram
  consumes for distillation — closing the loop of the memory architecture
  (episodic records feed semantic consolidation, each layer owning its role).

Success signal: engram distills from backscroll data through a defined
interface, and onboarding a new agent format is a reader plus a manifest, no
core changes.

## Permanently Out of Scope

- MCP server: rejected on verified token-cost evidence (4-32x CLI cost); the
  CLI + skill is the integration surface.
- Consolidation/summarization inside backscroll: episodic→semantic distillation
  is engram's territory per the identity decision. Backscroll records; engram
  learns.
