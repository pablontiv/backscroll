# Context pack: Refactor TOML minimalista (Claude+Pi) — Backscroll v0.2.3

> Nota de evidencia: en esta pasada no fue posible obtener nuevas referencias web reproducibles (la herramienta de búsqueda devolvió estado obsoleto del contexto); el análisis externo previo (pi config JSON) se conserva como evidencia resumida del pedido original.

## 1) High-signal state of codebase

### Config model + load path
- Canonical config shape in `src/config.rs`:
  - `Config { database_path, session_dirs: Vec<String>, embedding: EmbeddingConfig, sources: SourcesConfig }`.
  - `session_dirs` has `#[serde(alias = "session_dir", deserialize_with = "string_or_vec", default = "default_session_dirs")]`.
  - `string_or_vec` accepts either scalar string or array for session dirs and source dirs. See `src/config.rs:8-23`, `25-77`.
- Config file/env merge order is fixed in `Config::load()`:
  - `backscroll.toml` (cwd) -> `~/.config/backscroll/config.toml` -> `BACKSCROLL_*` env vars. `figment`-based extraction. See `src/config.rs:83-99`.
- Defaults and discovery helpers:
  - `Config::default_with_paths()` => `database_path = ~/.backscroll.db`, `session_dirs = ["."]` and default `embedding/sources` blocks. `src/config.rs:142-152`.
  - `discover_session_dirs()` reads `~/.claude/projects` for auto-discovery. `src/config.rs:101-140`.
- CLI uses fallback: if `Config::load()` fails, it uses `Config::default_with_paths()`. `src/main.rs:329-331`.

### CLI resolution and behavior
- `session_dir(s)` precedence in CLI path resolution:
  1) `--path`, 2) non-default `config.session_dirs`, 3) discovered Claude projects, 4) error. (`src/main.rs:294-318`)
- `sync`, `search`, `resume`, `status` currently call:
  - session parse via `parse_sessions`
  - plan sync via `sync_plans` (unless disabled)
  - external source sync via `SourceRegistry::from_config(&config.sources)` and `parse_all()`. (`src/main.rs:333-360`, `388-412`, `514-526`, `846-853`, `931+`).
- `SearchParams` includes `source`, `after`, `before`, `role`, `content_type`, `tag`, plus `hybrid`, `similarity_threshold`, `top_k`, `rrf_k`, but only some are consumed by DB search. (`src/core/mod.rs:126-160`, `src/main.rs:461-474`, `472-481`).

### Storage/search pipeline and schema touchpoints
- `search_items` table includes `source` and `source_metadata` (added via migration v6, currently not populated). See migrations and table defs in `src/storage/sqlite.rs:150-220`, `404-410`.
- Runtime source filtering in BM25 path only maps CLI values `sessions`/`plans` to DB values `session`/`plan`; other values pass through as-is but no mapping means no filtering for them (so `ke`, `decision`, etc. won’t be restricted). See `src/storage/sqlite.rs:526-550`.
- `sync_files` writes:
  - inserts into `search_items` with `INSERT ... source, source_path, ..., content_type`
  - updates `session_tags` only for `file.source == "session"`
  - optional embedding insertion if a provider exists. See `src/storage/sqlite.rs:620-733`.
- Embedding config is defined but not wired into runtime:
  - `EmbeddingConfig` parsed from TOML, default exists.
  - DB has embedding provider API but `create_engine` does not instantiate provider. (`src/config.rs:25-44`, `src/core/embedding.rs`, `src/storage/sqlite.rs:17-24`, `src/main.rs:288-293`).

### External source parsers
- External source types in `SourcesConfig`: `ke`, `decisions`, `memories`, `rules`, `specs`, `backlog` each with `string_or_vec` parsing. (`src/config.rs:47-63`, `306-321`).
- Source parsing lives in `src/core/sources.rs` with:
  - whole-document parsers (`parse_ke`, `parse_decision`, etc.) producing `ParsedFile { source: <type>, messages }`.
  - section parser for spec (`parse_sectioned_document`).
  - `SourceRegistry::parse_all()` scanning `.md` files under configured dirs and hash-deduping against `indexed_files`. (`src/core/sources.rs:156-211`).
- Fixtures include YAML frontmatter (`tests/fixtures/*`), and helper `parse_frontmatter` exists but is currently unused in parse flow. (`tests/fixtures/ke-001.md`, `tests/fixtures/spec-test.md`, `src/core/sources.rs:19-63`, `282-289`).

### DB/schema-related migration/compat surface
- Schema starts at v1 and migrates to v6.
- v6 adds `source_metadata` column + embedding tables. It is present but not used by query logic for manifest-like metadata. (`src/storage/sqlite.rs:404-410`, `1600+` migration + tests at `2235-2265`).

## 2) Relevant files to touch for strategy (read-only context)
- `src/config.rs` (config model + load order + session_dirs parsing): lines 1-152, 155-283.
- `src/main.rs` (CLI flags, path precedence, auto-sync, source pass-through): around 33-95, 288-360, 388-412, 467+, 524+.
- `src/storage/sqlite.rs` (schema, source filtering, sync/search/embeddings): around 150-260, 404-460, 620-733, 736-860, 958-975, 1358+.
- `src/core/sources.rs` (source parser registry + parse_frontmatter): 1-212, 280-343.
- `src/core/mod.rs` (SearchParams defaults/fields): 24-160.
- `src/core/embedding.rs` (embedding config + provider): 1-216.
- `tests/cli.rs` (session_dir env usage, source flags, no-embeddings behavior): especially around 12-36, 193-258, 1361-1364.
- `tests/lib_api.rs` (library search pipeline, KE filter usage): 13-149.
- `src/storage/sqlite.rs` tests for source filtering + migration: 1599-1705, 2259-2265, 2268+.
- Docs: `docs/configuration.md:14-42`, `README.md:193-209`, `backscroll.toml.example:1-9`, `CLAUDE.md:178-184`.

## 3) Existing patterns to preserve / reuse
- Figment + serde alias pattern (`alias = "session_dir"`, custom `deserialize_with`) is already used for backward compatibility.
- Path precedence and auto-discovery fallback used across commands; this pattern is expected to remain unless explicitly changed.
- `Indexed_files` hash-based incremental sync is central; manifest changes should avoid breaking hashing assumptions.
- Search output/commands already support per-source filtering via `--source` and pass-through to `SearchParams`.
- Public library API in `src/lib.rs` is shallow: `config`, `core`, `storage` modules only, so manifest changes in these modules are mostly ABI-safe unless exported types are altered.

## 4) Key inconsistencies / risks already present (important before new strategy)
- Docs/env mismatch:
  - Docs/sample use singular `session_dir` + `BACKSCROLL_SESSION_DIR`; one doc (`CLAUDE.md`) mentions `BACKSCROLL_SESSION_DIRS`. Code supports `session_dirs` as canonical and alias for TOML `session_dir`, and env provider likely supports either alias.
- CLI docs/exposed `--source` values imply more types than runtime filtering implements.
- `EmbeddingConfig` fields and embedding-related flags (`--similarity-threshold`, `--no-embeddings`) are underused/inert in runtime wiring.
- `parse_frontmatter` exists but not used, so markdown metadata in source files is not persisted/queried.

## 5) Concrete constraints for a TOML manifest refactor
- Strong backward compatibility is already needed for:
  - `session_dir` legacy singular key,
  - `session_dirs` array/scalar behavior,
  - env variables in tests.
- DB impact is likely low unless manifest metadata is persisted:
  - current schema supports adding metadata in `search_items.source_metadata` (v6), but feature is currently dormant.
- Keep sync auto-discovery behavior and file path resolution semantics stable for existing CLI defaults.
- Consider command behavior: many commands auto-sync on-demand; manifest changes should not alter this unexpectedly.

## 6) Validation/review artifacts already available
- `Config` tests: `test_config_with_file`, `test_config_session_dirs_array`, legacy scalar compatibility, defaults. (`src/config.rs:163-230`).
- CLI baseline tests rely on `BACKSCROLL_SESSION_DIR` and check search/commands, source filter sessions/plans, no-embeddings smoke. (`tests/cli.rs:14-34`, `193-258`, `1361+`).
- Storage migration and source-filter tests exist (`src/storage/sqlite.rs:1599-1705`, `2240+`).
- No existing tests specifically assert external source search filtering by exact `ke|decision|...` at DB level.

## 7) Open technical questions from current state
1. Should refactor introduce new manifest filename/section layout or only reshape existing `backscroll.toml`?
2. Should `source_metadata` begin being populated from markdown frontmatter in phase 1?
3. Is Claude+Pi consumption expected to be TOML-only in this phase or JSON also?
4. Resolve env naming/documentation contract (`BACKSCROLL_SESSION_DIR` vs `BACKSCROLL_SESSION_DIRS`) before publishing docs/examples.
5. Should `--no-embeddings`/`embedding` config become no-op for this pass, or should they be wired as part of refactor scope?
