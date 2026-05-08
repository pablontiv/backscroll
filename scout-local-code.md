# Code Context

## Files Retrieved
1. `src/config.rs` (lines 57-150, 153-297, 337-488) - current app config, embedded session input config, input-file loading, Claude discovery, tests.
2. `src/main.rs` (lines 15-353, 355-539, 540-930, 981-1063) - CLI flags/semantics, plan discovery, session input resolution, autosync callers, tests.
3. `src/core/sync.rs` (lines 1-159, 161-330, 333-658, 665-770) - current Claude/Pi parsers, noise filters, path/glob-ish discovery, parser registry, tests.
4. `src/core/models.rs` (lines 1-35) - Claude-shaped JSON models used by core parser/reader.
5. `src/core/mod.rs` (lines 15-160) - stable ingest/search boundary types (`ParsedFile`, `ParsedMessage`, `SearchParams`).
6. `src/core/reader.rs` (lines 1-40) - `read` command parser, currently Claude-only.
7. `src/core/plans.rs` (lines 1-55) - hardcoded plan source/role and section splitting.
8. `src/core/sources.rs` (lines 1-212) - hardcoded external source types/parsers and directory scanning.
9. `src/core/tagging.rs` (lines 1-82) - hardcoded session auto-tag heuristics.
10. `src/storage/sqlite.rs` (lines 520-665, 735-875, 1137-1326) - search filtering, role/source mappings, sync storage, analytics queries.
11. `src/lib.rs` (lines 1-5) - public module boundary.
12. `docs/configuration.md` (lines 1-83), `docs/sync.md` (lines 24-83), `backscroll.toml.example` (lines 1-26), `README.md` (lines 218-233) - documented current input config behavior and inconsistencies.
13. `tests/lib_api.rs` (lines 1-149), `tests/cli.rs` (lines 631-730, 1296-1345) - public API, role/content-type behavior tests.

## Key Code

### Current config is not separated from app config
`src/config.rs:57-99` defines `SessionInput` inside `Config` concerns:
```rust
pub struct SessionInput {
    pub source: String,
    pub parser: String,        // default hardcoded to "claude"
    pub paths: Vec<String>,
    pub include_agents: bool,
    pub active: bool,
}
```
`src/config.rs:136-150` keeps both app config and inputs in one `Config`:
```rust
pub struct Config {
    pub database_path: String,
    pub session_dirs: Vec<String>,
    pub embedding: EmbeddingConfig,
    pub sources: SourcesConfig,
    pub session_inputs: Vec<SessionInput>,
}
```
Input files loaded today: only `./backscroll.inputs.toml` and `./backscroll.inputs.d/*.toml` (`src/config.rs:153-207`). No `*.inputs.toml`, `claude.inputs.toml`, or `pi.inputs.toml` files exist in the repo.

### Current input manifest is shallow
Fields are only `source/parser/paths/include_agents/active`; no declarative field mappings, JSON selectors, role mapping, filters, globs, excludes, content block rules, or noise patterns (`src/config.rs:57-68`).

### Claude hardcoding
- Default parser is `claude`: `src/config.rs:71-86`.
- Auto-discovery uses `~/.claude/projects`: `src/config.rs:236-240`.
- CLI help says “Claude Code sessions”, subagents, and `~/.claude/plans`: `src/main.rs:17-35`.
- Plan discovery uses `~/.claude/plans` and hardcoded `.md/.markdown`: `src/main.rs:225-246`.
- `resolve_session_inputs()` converts CLI paths, legacy `session_dirs`, and discovered dirs to parser `claude`: `src/main.rs:299-346`.
- Claude parser is Rust code: record types `user|assistant`, `message.content`, tool block removal, `sessions-index.json`, `/sessions/` and `/subagents/`: `src/core/sync.rs:38-58`, `91-159`, `161-330`.
- `src/core/models.rs:17-35` names `ClaudeMessage` and models Claude JSON shape.
- `src/core/reader.rs:7-40` reimplements Claude-only parsing for `backscroll read`.

### Pi hardcoding
- Rust-native Pi parser lives in core: `parse_pi_value()` and `parse_pi_file()` (`src/core/sync.rs:333-504`).
- Registry hardcodes parser names to Rust structs: `src/core/sync.rs:520-529`.
- Pi role/content/timestamp/uuid selectors are hardcoded (`role`, `message.role`, `content`, `message.content`, `uuid`, `session_id`, `timestamp`): `src/core/sync.rs:423-488`.

### Filters/semantics hardcoded in core/storage/CLI
- Noise tags are static Rust regexes (`system-reminder`, `task-notification`, command tags, caveats): `src/core/sync.rs:13-36`, applied by `filter_noise()` at `61-82`.
- CLI validates only role `human|assistant` and content type `text|code|tool`: `src/main.rs:471-495`.
- Storage maps CLI role `human` to DB role `user`: `src/storage/sqlite.rs:559-565`.
- Storage source filter only maps `sessions` → `session`, `plans` → `plan`; every other source value becomes no filter: `src/storage/sqlite.rs:526-550`.
- Hybrid vector path does not apply `project/source/role/content_type/tag/date` filters to KNN candidates before RRF: `src/storage/sqlite.rs:774-843`.
- Session-only list/insights/validate/purge queries hardcode `source = 'session'`: `src/storage/sqlite.rs:1137-1158`, `1217-1299`, `1321-1326`.

### Stable integration boundary
`src/core/mod.rs:24-39` is the right normalization boundary to preserve:
```rust
pub struct ParsedMessage { role, text, ordinal, uuid, timestamp, content_type }
pub struct ParsedFile { source, source_path, hash, project, messages }
```
Desired declarative parsers should normalize into these types.

## Architecture
Current flow:

`Config::load()` merges app TOML/env and appends shallow session inputs → `main::resolve_session_inputs()` picks CLI `--path` > `session_dirs` > active inputs > `~/.claude/projects` fallback → `core::sync::parse_session_inputs()` dispatches to hardcoded Rust parsers (`claude`, `pi`) → parsers produce `ParsedFile`/`ParsedMessage` → `Database::sync_files()` stores rows and hardcoded session tags → search/list/insights apply hardcoded filters and session semantics.

Plans and external sources are separate ingestion paths, also hardcoded:
- Plans: `main::sync_plans()` discovers `~/.claude/plans`, calls `parse_plan()`, stores `source="plan"`.
- External sources: `SourcesConfig` in app config lists `ke/decisions/memories/rules/specs/backlog`, `SourceRegistry` scans `.md` and calls hardcoded parser functions.

## Current hardcoding to move toward `*.inputs.toml`
1. Parser selection/defaults: default `claude`, registry entries `claude`/`pi`.
2. Path roots/fallbacks: `~/.claude/projects`, `~/.claude/plans`, `session_dirs` as app config.
3. Globs/extensions/excludes: JSON/JSONL only, `.md/.markdown`, `/subagents/` exclude, max depth 3 for markdown sources.
4. Field mappings: record type, role, content, uuid, timestamp, session id, project inference.
5. Filters/transforms: record type filter, `isMeta`, tool block removal, noise regexes, Pi fallback text behavior.
6. Search semantics: role aliases (`human`→`user`), allowed content types, source aliases (`sessions`/`plans`), session-only analytics.
7. Source categories: `plan`, `ke`, `decision`, `memory`, `rule`, `spec`, `backlog` parser behavior.
8. Tagging patterns if “CLI-specific semantics” includes category heuristics.

## Risks / inconsistencies
- Docs show `source = "pi"` in some examples (`README.md:229-233`, `docs/configuration.md:60-65`), but `Config::active_session_inputs()` only accepts `source == "session"` (`src/config.rs:290-297`). `backscroll.toml.example` uses `source="session", parser="pi"` (`backscroll.toml.example:20-26`).
- `tests/lib_api.rs:141-148` claims `source: Some("ke")` filters KEs, but storage currently treats non-`sessions`/`plans` as no source filter (`src/storage/sqlite.rs:526-530`), so the test can pass for the wrong reason.
- `parse_sessions()` remains public and Claude-only (`src/core/sync.rs:647-658`), so library consumers are tied to Claude semantics.
- `backscroll read` bypasses the input parser registry and is Claude-only (`src/main.rs:532-539`, `src/core/reader.rs:7-40`).
- Moving filters to declarative config must account for hybrid search, otherwise vector results can bypass filters (`src/storage/sqlite.rs:774-843`).

## Start Here
Start with `src/core/sync.rs` and `src/config.rs`. Together they contain the current hardcoded parser registry, field mappings, path discovery, input config shape, and default Claude/Pi behavior. Keep `src/core/mod.rs` types as the normalization contract.

## Recommended next steps
1. Introduce an `input_config`/`inputs` module separate from `Config` that loads ordered `*.inputs.toml` files (including `claude.inputs.toml` and `pi.inputs.toml`) and normalizes legacy `session_dirs`/`backscroll.inputs.*` for compatibility.
2. Define a declarative input schema for: paths, globs/extensions, excludes, line format, record filters, field selectors, block extraction/exclusion, role aliases, timestamp/uuid/session/project selectors, source output, and noise regexes.
3. Replace `SessionInputParserRegistry::default()` hardcoded dispatch with a manifest interpreter, or use built-in TOML presets for `claude`/`pi` loaded through the same path.
4. Update `resolve_session_inputs()` so CLI `--path` wraps the configured default input definition instead of forcing `parser="claude"`.
5. Move Claude plans and external markdown sources into the same input model or explicitly separate “session inputs” from “document inputs” in the schema.
6. Make source/role/content filters data-driven and apply them consistently in BM25 and vector/hybrid paths.
7. Add regression tests for: `claude.inputs.toml`, `pi.inputs.toml`, arbitrary `*.inputs.toml` discovery order, legacy compatibility, source=`pi` vs source=`session` decision, role alias mapping, subagent exclude via TOML, and hybrid filter enforcement.

## Confidence
High for the identified hardcoding and integration points. Medium for the exact target schema because no `claude.inputs.toml`/`pi.inputs.toml` contract or examples exist locally.

## Gaps / open questions
- Should input files be root-level `*.inputs.toml`, under a directory, or both? Current loader only supports `backscroll.inputs.toml` and `backscroll.inputs.d/*.toml`.
- Should `source` in input config mean input provider (`pi`) or normalized DB source (`session`)? Current code and docs conflict.
- Should plans/external markdown sources be unified into input config now, or only session inputs?
- Are role/content/source filters considered app-level search config or input-level semantics?
- How much of project inference (`sessions-index.json`, path slugging) must be expressible declaratively vs retained as a built-in transform?
