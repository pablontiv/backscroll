# Meta-prompt for planning agent

> Nota de evidencia externa: no se revalidó via web en esta pasada (fallback del buscador devolvió estado del entorno); se basa en evidencia local + hallazgos de conversación previa del solicitante.

## Goal
Produce `implementation-strategy.md` as a **second-pass local-context design** for a **minimal TOML refactor** aimed at Claude+Pi compatibility, with **no code changes yet**. The strategy should be implementation-focused (not execution), include alternative designs, compatibility guarantees, schema impact, validation plan, and explicit open decisions.

## Context/evidence to carry forward
- Config is already loaded via Figment in `src/config.rs` with load order:
  - `backscroll.toml` (cwd) → `~/.config/backscroll/config.toml` → `BACKSCROLL_*` env (`Config::load`).
  - refs: `src/config.rs:83-99`.
- Canonical config field is `session_dirs: Vec<String>` with TOML alias `session_dir` and `string_or_vec` deserializer (string/array support).
  - refs: `src/config.rs:8-23`, `64-73`, tests `163-230`.
- CLI fallback is `Config::default_with_paths()` when load fails (i.e., runtime remains resilient).
  - refs: `src/main.rs:329-331`, defaults in `src/config.rs:142-152`.
- CLI path precedence: `--path` > explicit non-default `session_dirs` > discovered `~/.claude/projects` > error.
  - refs: `src/main.rs:294-318`.
- Existing manifest-like extension points:
  - Source parsing modules and config types for external sources (`SourcesConfig`, `SourceRegistry::parse_all`).
  - refs: `src/config.rs:47-63`, `src/core/sources.rs:156-211`, tests `src/core/sources.rs:301-343`.
- Search filtering implementation currently only maps `sessions`/`plans` in DB layer:
  - `source_filter` in `bm25_search()` maps only two aliases.
  - refs: `src/storage/sqlite.rs:526-550`.
- Embedding config/flags are currently largely disconnected:
  - `embedding` config parsed, but not injected into runtime; search `similarity_threshold/top_k/rrf_k` not consumed in actual search path.
  - refs: `src/config.rs:25-45`, `src/core/mod.rs:136-160`, `src/main.rs:93-95,372-475`, `src/storage/sqlite.rs:760+`.
- `source_metadata` column exists in schema v6 but not populated/read yet.
  - refs: `src/storage/sqlite.rs:404-410`, `2242-2245`.
- Docs/env drift exists:
  - docs use singular `session_dir` and `BACKSCROLL_SESSION_DIR` while one doc lists `BACKSCROLL_SESSION_DIRS`, and example TOML still singular.
  - refs: `backscroll.toml.example:9`, `docs/configuration.md:16-42`, `README.md:193-209`, `CLAUDE.md:181-184`.

## Success criteria for completed strategy
1. `implementation-strategy.md` includes:
   - comparison of implementation options (at least 2–3),
   - proposed module deltas (new/modified modules),
   - manifest schema proposal (fields, types, defaults, validation),
   - backward-compatibility matrix (legacy keys + env + default behaviors),
   - DB/schema impact assessment with migration/no-migration decision,
   - validation plan and command list.
2. Explicitly call out which decisions are in-scope vs out-of-scope for this second pass.
3. Include concrete open questions for product/API owner and concrete assumptions.
4. Be consistent with existing code contracts: multi-path `session_dirs`, CLI precedence, auto-sync semantics.

## Hard constraints
- Preserve existing compatibility path unless explicitly de-scoped and justified:
  - legacy `session_dir` support,
  - existing env fallback behavior used in tests,
  - default paths and auto-discovery fallback.
- Do **not** require DB schema changes unless strategy decides and justifies it.
- No functional regression in current defaults/tests in a first phase.
- Keep docs/authorship references to existing behaviors and explicitly correct the `session_dir`/`session_dirs` naming confusion.

## Suggested approach (for author of strategy)
- Option A (minimal): extend current TOML file format/documentation only (no new loader), add a top-level `manifest` block for Claude+Pi intent, keep legacy aliases.
- Option B: introduce a dedicated `manifest` module + typed structs that normalize legacy `Config` into an internal manifest model; keep wire format stable.
- Option C: new file precedence layer (e.g., `backscroll.manifest.toml`) with fallback to existing `backscroll.toml`, if Claude+Pi ecosystem requires distinct file naming.
- For each option, explicitly score:
  - implementation effort,
  - backward compatibility risk,
  - test surface,
  - future ability to persist metadata into `source_metadata`.

## Validation plan to include
- Config/unit:
  - `cargo test src/config.rs` style expectations:
    - `test_config_with_file`, `test_config_session_dirs_array`, `test_config_session_dir_legacy_string`, defaults.
- CLI/integration:
  - `cargo test test_cli_*` critical paths (at least status/help/search/source flags/sync path resolution/no-embeddings smoke).
- Storage/search behavior:
  - `cargo test test_search_source_filter_*`, `test_schema_v6_migration`.
- If manifest schema changes parsing only:
  - add focused tests for new invalid/missing/legacy cases in `src/config.rs` and relevant parser tests.
- Add/adjust docs checks by inspection and explicit consistency review of `README.md` + `docs/configuration.md` + `backscroll.toml.example`.

## Stop/escalation rules
- Escalate/ask owner if any of these are undecided:
  - Is Claude+Pi consumption expected to be TOML-only in this phase or JSON also?
  - Should `source` filtering for external source types be made strict in this same pass?
  - Is env naming contract `BACKSCROLL_SESSION_DIR` vs `BACKSCROLL_SESSION_DIRS` to be standardized now?
  - Should unused embedding controls be explicitly deferred (documented as known debt) or fixed in-phase with TOML manifest work?

## Resolved questions and assumptions
- A practical first pass can likely ship with **zero DB schema migration** by using existing `source_metadata` only as a future field.
- Existing source filters in CLI/docs are partially implemented; strategy should call out this gap but does not need to implement it unless explicitly included.
- Current implementation is already close to supporting richer config structures, so a “minimal” TOML refactor should prioritize parser/docs compatibility and avoid deeper search/DB logic shifts unless required.

## Suggested command validation list (for the next agent)
- `cargo test test_config_with_file test_config_session_dirs_array test_config_session_dir_legacy_string`
- `cargo test test_config_session_dirs_default_when_omitted`
- `cargo test test_search_source_filter_plans_only test_search_source_filter_sessions_only`
- `cargo test test_schema_v6_migration`
- `cargo test test_no_embeddings_flag`
- `cargo test` (full regression after doc/config changes are implemented)
