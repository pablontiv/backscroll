# Declarative Input Engine Tradeoffs

## Evidence files retrieved
1. `src/config.rs` (lines 57-116, 153-236, 290-296, 337-405) - `SessionInput`, manifest loading from `backscroll.inputs.*`, active input filtering, config tests.
2. `src/main.rs` (lines 294-367, 390-404, 450-499) - precedence (`--path` > `session_dirs` > inputs > fallback), sync dispatch, source filter creation.
3. `src/core/sync.rs` (lines 14-88, 91-159, 161-303, 333-392, 394-505, 507-648, 693-715) - noise filtering, project inference, Claude/Pi parsers, registry, parser tests.
4. `src/core/models.rs` (lines 3-31) and `src/core/mod.rs` (lines 24-39, 125-133) - parser input/output IR (`SessionRecord`, `ParsedFile`, `ParsedMessage`, `SearchParams`).
5. `src/storage/sqlite.rs` (lines 178-187, 274-289, 405-443, 526-575, 608-728, 1028-1044, 1138-1158, 1289-1300, 1321-1369) - DB schema, source/content filters, sync insert path, tagging, embeddings, session-only APIs.
6. `src/core/sources.rs` (lines 66-135, 156-209) and `src/core/plans.rs` (lines 7-47) - existing non-session source registry and markdown splitting model.
7. `docs/sync.md` (lines 24-59), `README.md` (lines 198-234), `docs/roadmap/O01-session-input-refactor/README.md` (lines 7-30), `T006-add-pi-input-support.md` (lines 21-37) - stated declarative input goals and docs.

## Current shape
- The repo already has a native parser registry for sessions: `claude` and `pi` are registered in `SessionInputParserRegistry::default()` and dispatched by `input.parser()` (`src/core/sync.rs:507-648`).
- The DB boundary is generic enough: any engine that emits `ParsedFile { source, source_path, hash, project, messages }` and `ParsedMessage { role, text, ordinal, uuid, timestamp, content_type }` can use `Database::sync_files` unchanged (`src/core/mod.rs:24-39`; `src/storage/sqlite.rs:608-728`).
- But several downstream features assume sessions use `source = 'session'`: auto-tagging, list/insights totals, validation, and source-specific queries (`src/storage/sqlite.rs:647-664`, `1138-1158`, `1289-1300`, `1321-1326`). The roadmap also calls this invariant out (`docs/roadmap/O01-session-input-refactor/README.md:18-22`).

## Option A: generic JSONL mapping/filter engine with TOML presets
**Pros**
- Best for simple event streams: Pi parsing today is already dynamic `serde_json::Value` with field fallbacks for `role`, `message.content`/`content`, `uuid`, `timestamp` (`src/core/sync.rs:394-488`). Those rules could become TOML presets.
- Strong fixture-driven testability: manifests + JSONL fixtures can assert exact `ParsedFile`/`ParsedMessage` output without DB setup; current tests already cover a minimal Pi fixture (`src/core/sync.rs:693-715`).
- Can reduce Rust changes for new JSONL variants if mappings are limited to field paths, defaults, predicates, content extraction, and content-type rules.

**Risks / costs**
- Claude is not just field mapping. Existing behavior includes typed `SessionRecord`, top-level record-type filtering, `isMeta` skipping, block flattening/excluding tool blocks, noise stripping, per-file SHA dedupe, `sessions-index.json` project inference, and subagent exclusion (`src/core/sync.rs:14-303`). A TOML engine needs either a real expression language or built-in functions for these cases.
- Validation becomes a product surface: field paths, filters, arrays, defaults, and content-type classifiers must fail clearly. Current unknown parsers and bad files mostly warn/skip (`src/core/sync.rs:232-234`, `543-550`, `569-578`), and config parse failures in input files are ignored with warnings (`src/config.rs:157-205`). A generic engine should probably fail fast on invalid manifests while continuing per-file parse failures with counts.
- Performance likely regresses for Claude if everything goes through `serde_json::Value`; the typed parser currently only materializes a narrow schema (`src/core/models.rs:3-31`). Hashing already reads whole files and parsing reads them again, so IO may dominate, but generic dynamic filters add CPU on large corpora.
- DB risk: `uuid` is globally `UNIQUE` and inserts are `INSERT OR IGNORE` (`src/storage/sqlite.rs:178-187`, `628-642`). A generic mapping that maps a session id as every message uuid can silently drop rows or break embedding lookup by `(source_path, ordinal)`. Manifest validation/presets must distinguish message uuid vs session id.

**Implication**: viable as an additional parser (`parser = "jsonl_map"`) for simple JSONL sources, not as a full replacement for Claude without re-implementing many native hooks in the declarative language.

## Option B: keep parser registry + declarative discovery/presets
**Pros**
- Lowest migration risk: the existing registry and `SessionInput` config already provide the seam (`src/config.rs:57-116`; `src/core/sync.rs:507-648`). Extend discovery/globs/presets without changing `Database::sync_files`.
- Preserves complex native behavior for Claude and any future complex adapters while allowing a generic JSONL adapter for common cases.
- Testability stays straightforward: unit-test each parser plus resolver precedence (`src/main.rs:294-367`; config tests at `src/config.rs:337-405`).
- DB/schema impact is minimal if all session adapters continue to emit `source="session"` and use existing `ParsedFile` fields.

**Risks / costs**
- New complex adapters still require Rust changes/release unless they fit the generic preset adapter.
- Current config docs and code have a mismatch: README shows `source = "pi"` (`README.md:229-233`), but `active_session_inputs()` only accepts `source == "session"` (`src/config.rs:290-296`). The Pi test uses `source="session"` (`src/core/sync.rs:703-705`). This must be clarified before adding more presets.
- Current CLI advertises source filters for `ke`, `decision`, etc. (`src/main.rs:65-67`), but SQLite only maps `sessions` and `plans` specially and ignores other source values (`src/storage/sqlite.rs:526-529`). A generic input engine will expose this more.

**Implication**: recommended path. Keep native `claude`/`pi` registry, add declarative discovery and a constrained `jsonl_map` parser for presets. Treat `source` as indexed output kind and add a separate `adapter`/`parser`/metadata concept if needed.

## Option C: plugin/script adapters
**Pros**
- Maximum flexibility: adapters can normalize arbitrary formats to the `ParsedFile`/`ParsedMessage` IR.
- Avoids inventing a large TOML expression language.

**Risks / costs**
- Highest operational/security risk: no plugin runtime or sandbox dependency is present (`Cargo.toml` only has core libs like `figment`, `serde_json`, `walkdir`, `rusqlite`, `sqlite-vec`). External processes introduce versioning, stderr/protocol handling, timeouts, path permissions, and reproducibility issues.
- Harder incremental sync semantics: who computes hash, source_path stability, ordinals, and adapter version invalidation?
- More fragile error handling than native parsers; sync currently assumes parser output is in-process `ParsedFile` and wraps DB writes in one transaction (`src/storage/sqlite.rs:608-728`).

**Implication**: keep out of first migration. If later needed, make plugins produce a strict JSON IR matching `ParsedFile` and include adapter-version/config fingerprinting.

## DB/schema impact
- No required schema change for Options A/B if output stays `ParsedFile`/`ParsedMessage`.
- Consider using existing `source_metadata` (`src/storage/sqlite.rs:405-410`) for adapter name/version/config hash, but it is not currently populated.
- Reindex is safer than in-place migration because text extraction, ordinals, uuid mapping, and content_type may change. User backward compatibility may not matter, but DB invariants do.

## Decision implications
- Choose **B + constrained A**: native parser registry remains the safety net; add a generic JSONL mapping parser for simple sources and TOML presets.
- Define a manifest contract with strict validation before parsing: allowed parser names, output `source`, field paths, uuid semantics, content_type enum, role mapping, timestamp format, and unknown-field policy.
- Keep all conversational adapters emitting `source="session"` unless downstream session/list/insight behavior is intentionally redesigned.
- Fix or decide the `source="pi"` docs/code mismatch early.

## Confidence and gaps
- Confidence: **medium-high** for code-level tradeoffs and DB risks based on local evidence.
- Gaps: no sample real Pi logs found; did not run benchmarks; did not inspect full CLI source-filter tests; product owner says user backward compatibility is not needed, but local roadmap/docs still mention preserving legacy semantics.