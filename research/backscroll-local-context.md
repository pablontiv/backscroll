# Code Context

## Files Retrieved
1. `README.md` (lines 7-11, 30-62, 114-169, 173-223, 225-284) - product positioning, install/runtime input preset model, quick start, CLI/AI-native/config surfaces.
2. `Cargo.toml` (lines 1-52) - crate/bin identity, static CLI intent, dependencies for SQLite/FTS/vector/JSONPath/CLI.
3. `src/main.rs` (lines 24-207, 246-268, 441-576, 577-967) - public CLI surface, engine creation, sync/autosync, retrieval/analytics command behavior.
4. `src/core/mod.rs` (lines 9-134) - normalized domain model and `SearchEngine` trait/API boundary.
5. `src/input_config.rs` (lines 23-154, 341-529) - user-scoped manifest schema, loader, and validation rules.
6. `src/core/sync.rs` (lines 61-108, 261-416, 871-1081) - generic discovery, JSON/JSONL/markdown parsing, predicate filtering, normalization, hash-based parse boundary.
7. `src/storage/sqlite.rs` (lines 57-119, 121-449, 515-612, 660-908, 1016-1444) - SQLite opening/read-only integration, schema, filters/BM25, sync/search implementation, stats/ops/analytics.
8. `src/core/embedding.rs` (lines 7-48, 52-118, 157-211) - embedding provider abstraction and ONNX provider implementation.
9. `src/core/chunking.rs` (lines 1-79) and `src/core/hybrid.rs` (lines 1-35) - chunking and Reciprocal Rank Fusion support.
10. `src/core/tagging.rs` (lines 1-80) - heuristic session auto-tags.
11. `src/output.rs` (lines 4-58) - text/JSON/robot output and token limiting.
12. `src/core/sources.rs` (lines 1-204) and `src/core/plans.rs` (lines 1-48) - legacy/source-specific markdown parsers still present, plus plan section splitting.
13. `inputs/claude.inputs.toml` (lines 1-54) and `inputs/pi.inputs.toml` (lines 1-38) - shipped provider manifests.
14. `docs/input-contract.md` (lines 1-38, 41-268, 271-350), `docs/sync.md` (lines 1-115), `docs/search.md` (lines 1-82), `docs/read.md` (lines 1-38), `docs/configuration.md` (lines 1-91) - canonical contract and user workflows.
15. `docs/intention-agentic-input-definitions.md` (lines 1-65) - explicit invariants and no-goals.
16. `docs/roadmap/O02-generic-agnostic-input-engine/README.md` (lines 1-73), `docs/roadmap/O03-global-user-scoped-inputs/README.md` (lines 1-78) - roadmap intent toward generic, global manifests.
17. `tests/cli.rs` (lines 158-214, 228-351, 533-721, 737-965, 1039-1266, 1314-1715, 1757-2804) - integration coverage for CLI, manifests, Pi/Claude fixtures, filters, analytics, ops.

## Key Code

- **Backscroll's stable ingestion boundary** is small and provider-neutral:

```rust
// src/core/mod.rs:9-29
pub struct SearchResult { pub source_path: String, pub text: String, ... }
pub struct ParsedMessage { pub role: String, pub text: String, pub ordinal: usize, ... }
pub struct ParsedFile { pub source: String, pub source_path: String, pub hash: String, pub project: Option<String>, pub messages: Vec<ParsedMessage> }
```

- **Programmatic/API boundary** is the `SearchEngine` trait (`src/core/mod.rs:108-134`): `sync_files`, `search`, `get_file_hashes`, `clear_hashes`, `purge`, `get_stats`, `get_session_id`, `get_topics`, `list_sessions`, `get_project_breakdown`, `validate`, `optimize_fts`, `get_insights`.
- **CLI surface** (`src/main.rs:24-207`) includes `sync`, `search`, `resume`, `list`, `topics`, `reindex`, `purge`, `validate`, `insights`, `export`, `inputs {list,validate,test}`, and `status`. Public `read` is intentionally removed/tested absent.
- **Autosync is part of retrieval behavior**: `search`, `resume`, `list`, `topics`, `insights`, `export`, and `status` create the engine and call `sync_manifest_inputs` before query/metrics (`src/main.rs:475-576`, `577-967`).
- **Generic input manifests** are loaded only from `<config_dir>/backscroll/inputs/*.inputs.toml`, with `BACKSCROLL_CONFIG_DIR` override (`src/input_config.rs:341-417`). Active manifests validate globs, JSONPath selectors, UTF-8, required mapping/content for JSON/JSONL, and text removal regexes (`src/input_config.rs:418-529`).
- **Parser pipeline** is data-driven: glob discovery (`src/core/sync.rs:61-108`), record/content predicates and JSONPath extraction (`261-416`), markdown whole/sectioned parsing and dry-run (`871-970`), then `ParsedFile` emission with SHA-256 hash and project defaulting (`971-1081`).
- **Storage/indexing** is local SQLite: `indexed_files`, `search_items`, external-content FTS5 `messages_fts`, `messages_vocab`, `dynamic_stopwords`, `session_tags`, `chunks`, `vec_embeddings`, `embedding_metadata` (`src/storage/sqlite.rs:121-449`). `Database::open_readonly()` is a likely integration point for external consumers (`src/storage/sqlite.rs:90-119`).
- **Search behavior**: query sanitization removes dynamic stopwords, quotes terms, adds prefix matching, and uses FTS5 BM25/snippets plus generic filters for project/source/source_path/date/role/content_type/tag (`src/storage/sqlite.rs:515-612`). Hybrid vector search exists in storage (`660-908`) but falls back to BM25 if no embedding provider/chunks are present (`799-810`).
- **Outputs for agents/scripts**: JSON lines and compact tab-separated `--robot`, with approximate `--max-tokens`, are implemented in `src/output.rs:4-58` and documented as AI-native in `README.md:208-223`.

## Architecture

Backscroll is a local retrieval layer, not just a Pi/Claude parser:

```text
Global input manifests -> discover/decode/filter/map/text normalize -> ParsedFile/ParsedMessage
  -> SQLite search_items + FTS5 + hashes + tags (+ optional chunks/vector tables)
  -> CLI/API search/list/topics/resume/status/validate/export with autosync
```

Current responsibilities and distinctive capabilities:

- **Local archive/index ownership**: creates/maintains a local SQLite DB, default `~/.backscroll.db` (`README.md:225-245`, `src/config.rs:169-188`).
- **Incremental sync**: hashes every input file and skips unchanged files (`README.md:158-160`, `src/core/sync.rs:971-1016`, `src/storage/sqlite.rs:660-782`).
- **Provider-neutral ingestion**: Claude and Pi are presets, not hardcoded runtime parsers; both emit `source = "session"` (`docs/input-contract.md:1-38`, `inputs/claude.inputs.toml:1-54`, `inputs/pi.inputs.toml:1-38`).
- **Noise/filter rules in TOML**: Claude removes reminders/task notifications/caveats/command output via `inputs.text.remove`; Pi includes only text blocks (`inputs/claude.inputs.toml:40-54`, `inputs/pi.inputs.toml:27-38`).
- **Document/source indexing**: plans and external markdown (`plan`, `ke`, `decision`, `memory`, `rule`, `spec`, `backlog`) are now represented as declarative inputs, though legacy parser modules still exist (`docs/input-contract.md:311-350`, `src/core/sources.rs:1-204`).
- **Search ergonomics**: source path lookup replaces direct read (`docs/read.md:1-38`), and filters cover source/project/path/date/role/content/tag (`src/main.rs:39-100`, `src/storage/sqlite.rs:536-577`).
- **Operational commands**: validation, purge, reindex, FTS optimize, status, project breakdown, topics, insights, export are in CLI and trait (`src/main.rs:102-207`, `src/storage/sqlite.rs:962-1444`).
- **AI/tool workflows**: `--robot`, `--json`, deterministic output, and token caps are first-class (`README.md:187-223`, `src/output.rs:4-58`).

Likely users/workflows:

- Developers or AI agents searching prior Claude Code/Pi sessions locally.
- Automation/LLM tools consuming `backscroll search --robot --max-tokens ...`.
- Operators checking index health with `status`/`validate`, pruning with `purge`, or reindexing manifests.
- External Rust consumers opening the DB read-only or using the library crate (`src/lib.rs:1-5`, `src/storage/sqlite.rs:90-119`).

## Pi-memory supersession assessment

From the local codebase perspective, **pi-memory could supersede Backscroll only if it replaces a full local indexing/retrieval subsystem**, not merely Pi session memory storage.

An external Pi memory extension would need to replace or provide compatibility for:

1. **CLI contract**: commands/flags and output shapes used by scripts and agents: `sync`, `search`, `resume`, `list`, `topics`, `status`, `inputs validate/list/test`, `--json`, `--robot`, `--max-tokens`, `--source`, `--source-path`, filters, exit behavior.
2. **Autosync semantics**: retrieval commands index active manifests before querying.
3. **Manifest/config model**: global user-scoped `*.inputs.toml`, installed Claude/Pi presets, no project-local manifests, no `backscroll.toml` ingestion routes.
4. **Data identity**: `source = "session"` for both Claude and Pi, `source_path`, `uuid`, timestamp, project, ordinal, content_type, tag semantics.
5. **SQLite-backed retrieval**: FTS5/snippet/BM25 search, dynamic stopwords, source/date/role/path/content/tag filters, path lookup, stats and validation tables.
6. **Document-source scope**: Backscroll indexes more than Pi memory: Claude sessions, Pi sessions, plans, and arbitrary markdown knowledge/decision/memory/rule/spec/backlog inputs.
7. **Integration points**: read-only DB access and public Rust crate boundaries if any downstream tools depend on them.
8. **Install/test expectations**: preset installation without overwriting user edits; tests assert no fallback to legacy session dirs and no public `read` command.

Important local caveat: embedding/hybrid infrastructure exists, but the CLI currently does not wire an `OnnxProvider` into `create_engine` (`src/main.rs:246-250`), and `--no-embeddings` is parsed but ignored in command handling (`src/main.rs:475-476`, `752-753`). Storage therefore normally falls back to BM25 unless a provider is injected in tests or by a future caller (`src/storage/sqlite.rs:799-810`). Do not assume production vector search is active from the local code alone.

## Gaps / open questions

- No local `pi-memory` code was present in this repository, so feature parity cannot be verified here.
- Unknown whether external users depend on the SQLite schema or `Database::open_readonly`; the code exposes it but this repo does not show downstream consumers.
- Documentation/status differs across inherited project notes vs current README; current local code/README emphasize BM25/full-text with optional-but-unwired vector infrastructure.
- Roadmap O03 is marked pending while much of its behavior appears implemented/tested; confirm release state before deprecating Backscroll.

## Confidence level

- **High** for Backscroll's current local responsibilities, CLI/config/storage/API surfaces, and tests: directly evidenced in code/docs.
- **Medium** for likely external integration points: inferred from public CLI, library exports, read-only DB API, and tests.
- **Low** for whether pi-memory can actually supersede it: pi-memory implementation/capabilities were outside this local repository.

## Decision implication

Treat pi-memory as a **possible replacement only with an explicit compatibility/migration plan**. Backscroll already supports Pi session ingestion via manifests and also covers Claude sessions, markdown knowledge sources, local SQLite FTS search, operational commands, and AI-friendly outputs. If pi-memory only stores/retrieves Pi memories, it is more likely a complementary input/source than a full supersession. Full replacement means matching the CLI/API/storage semantics above or providing shims and a DB/index migration path.

## Start Here

Open `src/main.rs` first. It shows the user-visible contract and autosync behavior that any pi-memory replacement would need to preserve or intentionally break.
