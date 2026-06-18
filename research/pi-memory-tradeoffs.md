I did not write `/home/shared/harness/backscroll/research/pi-memory-tradeoffs.md` because the task also said “Do not edit files,” and the higher-priority runtime instruction says no-edit wins. Findings:

# Research: Backscroll vs Pi memory

## Summary
Do **not** replace Backscroll with `@samfp/pi-memory` alone. Pi memory is best viewed as a **preference/lesson injector** for Pi sessions, while Backscroll is a **provider-neutral CLI search/indexing layer** for session archives and external sources. A safer path is to pilot Pi memory—or `pi-total-recall`—as a complement, then validate parity against Backscroll before deemphasizing.

## Findings

1. **Different primary jobs: memory vs archive search** — `pi-memory` learns preferences, project patterns, and corrections, then injects an 8KB `<memory>` block into future Pi prompts. Its own README says it “complements session-search”: session-search finds “what you did,” memory remembers “what you learned.” This directly argues against using `pi-memory` alone as a Backscroll replacement. [pi-memory package](https://pi.dev/packages/@samfp/pi-memory?name=memory), [repo](https://github.com/samfoy/pi-memory)

2. **Backscroll has broader retrieval scope** — Backscroll is a standalone Rust CLI indexing local sessions and external source types into SQLite with BM25/vector/RRF search, machine-readable output, filters, and provider-neutral input manifests. It also ships as a static binary and can be used outside Pi. [Backscroll README](https://github.com/pablontiv/backscroll), [Cargo metadata](https://github.com/pablontiv/backscroll/blob/master/Cargo.toml)

3. **`pi-memory` is not vector/hybrid search** — Its store uses SQLite tables `semantic`, `lessons`, and `events`; search is FTS5 BM25 when available, otherwise substring fallback. There is no embedding/vector index in `pi-memory` itself. [store.ts](https://github.com/samfoy/pi-memory/blob/main/src/store.ts)

4. **Pi ecosystem has closer Backscroll analogs than `pi-memory`** — `pi-session-search` provides Pi session indexing with FTS5 and optional hybrid embeddings + RRF; `pi-knowledge-search` provides hybrid vector + BM25 over local files; `pi-total-recall` bundles memory, session search, and knowledge search. If replacing/deemphasizing Backscroll, compare against **pi-total-recall**, not memory alone. [pi-session-search](https://github.com/samfoy/pi-session-search), [pi-knowledge-search](https://github.com/samfoy/pi-knowledge-search), [pi-total-recall](https://github.com/samfoy/pi-total-recall)

5. **Privacy/offline tradeoff is non-trivial** — Backscroll can stay local/offline after model setup. `pi-memory` stores SQLite locally, but consolidation runs a background `pi -p --print` LLM call at shutdown; default model is Anthropic unless configured, though local/Ollama-compatible models can be used. Failed consolidation is swallowed silently. [index.ts](https://github.com/samfoy/pi-memory/blob/main/src/index.ts), [README](https://github.com/samfoy/pi-memory)

6. **Edge cases where `pi-memory` underperforms Backscroll** — Short sessions with fewer than ~3 user messages are not consolidated; extraction intentionally rejects file paths, code snippets, debugging recipes, activity summaries, and derivable project structure—exactly the kind of material Backscroll often retrieves from raw logs. [consolidator.ts](https://github.com/samfoy/pi-memory/blob/main/src/consolidator.ts)

7. **Pi extension/package risk** — Pi packages/extensions run with full system access and can influence agent behavior, so third-party source review and version pinning matter. Pi docs explicitly warn that packages/extensions can execute arbitrary code. [Pi packages docs](https://pi.dev/docs/latest/packages), [Pi extensions docs](https://pi.dev/docs/latest/extensions)

8. **API/schema stability risk** — `@samfp/pi-memory` is recent (`1.0.4`, published May 2026) and depends on Pi extension lifecycle hooks like `session_start`, `before_agent_start`, `agent_end`, and `session_shutdown`. Pi session format is versioned and migrates across versions, so extension compatibility should be treated as an operational dependency. [package page](https://pi.dev/packages/@samfp/pi-memory?name=memory), [Pi session format](https://pi.dev/docs/latest/session-format)

9. **Maintenance cost shifts, not disappears** — Replacing Backscroll with Pi packages reduces Rust/search-engine maintenance, but adds npm/Pi runtime maintenance, package review, config drift, DB backup/migration, and provider/model configuration. Backscroll maintenance remains higher for hybrid search, cross-platform binaries, and external-source parsing, but gives more control and portability.

## Option comparison

| Option | Pros | Risks |
|---|---|---|
| Keep Backscroll only | Portable CLI, local SQLite, provider-neutral, external-source indexing | Maintains custom Rust/search stack |
| Add `pi-memory` | Low effort; improves future Pi behavior with preferences/corrections | Not archive search; LLM consolidation/privacy risk |
| Add `pi-total-recall` | Closest Pi-native overlap: memory + session + knowledge search | Pi-only; package/runtime trust; parity still unproven |
| Replace Backscroll | Less internal maintenance | High loss risk: CLI, Claude/Pi neutrality, external source semantics, offline guarantees |

## Validation approach

1. Build a parity test set: preference recall, exact error/path lookup, prior decision retrieval, external rule/spec retrieval, cross-project search, offline mode.
2. Run Backscroll vs `pi-memory` vs `pi-total-recall` on the same tasks.
3. Measure recall@5, MRR, answer correctness, latency, context injected, consolidation failures, false/stale memories, and privacy/model calls.
4. Shadow-run for 2–4 weeks before decommissioning anything.
5. Require version pinning, DB backup/export, and source review before adopting third-party Pi packages.

## Confidence
**Medium-high** for functional comparison from direct docs/source. **Medium** for operational risk because no live install/parity benchmark was run.

## Gaps
No empirical benchmark on this Backscroll corpus; no confirmed migration path from Backscroll indexes to Pi memory/session-search; no test of Pi package behavior under local Node/Pi versions.

## Recommended next step
Pilot **`pi-memory` as a complement**, not replacement. If the goal is maintenance reduction, evaluate **`pi-total-recall` vs Backscroll** with a parity matrix and keep Backscroll until Pi tools pass the archive-search and external-source cases.