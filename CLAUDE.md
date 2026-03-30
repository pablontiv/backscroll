# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Backscroll is a Rust CLI tool that indexes Claude Code sessions, plans, and external knowledge sources into SQLite for hybrid search (BM25 + vector embeddings via sqlite-vec + RRF fusion). It treats sessions as an event store with incremental sync via SHA-256 deduplication.

**Status**: 1.0 — Hybrid search engine complete (BM25 + embeddings + RRF). External source types (KEs, decisions, memories, rules, specs, backlog). >95% test coverage. Requires Rust 1.85+.

## Build & Test Commands

```bash
just check              # rustfmt --check + clippy -D warnings + cargo check
just test               # cargo test --all-features
just fmt                # cargo fmt --all
just build              # cargo build --release
just static-build       # cargo zigbuild (Linux musl static binary)
just coverage-summary   # LLVM coverage report
just audit              # cargo deny check licenses bans
```

Run a single test: `cargo test test_name`

Tests use `assert_cmd` + `predicates` for CLI integration and `insta` for snapshot tests. Unit tests are co-located in each module. Integration tests in `tests/cli.rs`. Update snapshots with `cargo insta review`.

Install script tests: `bash tests/test-install.sh` (bash) and `Invoke-Pester tests/test-install.ps1` (PowerShell). Both run in CI — static checks in `check-lint`, runtime PowerShell tests in a Windows job.

## Architecture

### Module Layout

```
main.rs (CLI: clap)
├── config.rs          — Figment-based config (TOML files + env vars + discovery)
├── output.rs          — Formats output (Text, JSON, Robot) and limits tokens
├── core/
│   ├── mod.rs         — SearchResult struct, SearchEngine trait (port)
│   ├── models.rs      — SessionRecord wrapper, MessageContent untagged enum
│   ├── plans.rs       — Markdown plan parser (split by ## headers)
│   ├── reader.rs      — Direct reading of single filtered session files
│   ├── sync.rs        — WalkDir, hashing, parsing JSONL with noise filtering
│   ├── tagging.rs     — Heuristic session auto-tagging (regex-based category detection)
│   ├── embedding.rs   — EmbeddingProvider trait, MockProvider, OnnxProvider
│   ├── chunking.rs    — Text chunking pipeline (~512 tokens, sentence-aware)
│   ├── sources.rs     — External source parsers (KE, decision, memory, rule, spec, backlog) + SourceRegistry
│   └── hybrid.rs      — Reciprocal Rank Fusion (RRF) for combining BM25 + vector rankings
└── storage/
    └── sqlite.rs      — SQLite adapter (FTS5 + sqlite-vec, hybrid search, embeddings)
```

- `main.rs` — CLI entrypoint (clap `Parser`/`Subcommand`), command dispatch, plan sync orchestration
- `config.rs` — Figment-based config resolution (TOML files + env vars), multi-path session dirs, auto-discovery of `~/.claude/projects/`
- `output.rs` — Output formatter (Text, JSON, Robot) with approximate token limiting
- `core/mod.rs` — Domain types (`SearchResult`, `ParsedFile`, `Stats`) and `SearchEngine` trait (port)
- `core/models.rs` — `SessionRecord` wrapper, `MessageContent` untagged enum for defensive parsing
- `core/plans.rs` — Markdown plan parser: splits `~/.claude/plans/*.md` by `##` headers into indexable sections
- `core/reader.rs` — Direct reading and filtering of individual session files
- `core/sync.rs` — WalkDir traversal, SHA-256 hashing, JSONL parsing, noise filter regex (LazyLock), content-type classification
- `core/tagging.rs` — Heuristic session auto-tagging: regex patterns detect categories (debugging, refactoring, feature, testing, docs, config)
- `storage/sqlite.rs` — SQLite adapter (external FTS5 with Porter stemmer, triggers, BM25 ranking, WAL mode, source-aware filtering, session tags)

Twelve CLI commands: `sync [--path] [--include-agents] [--no-plans] [--optimize]`, `search <query> [--project] [--all-projects] [--json] [--robot] [--fields] [--max-tokens] [--source] [--after] [--before] [--role] [--limit] [--offset] [--content-type] [--tag]`, `read <path>`, `resume <query> [--project] [--all-projects] [--robot] [--source]`, `topics [--project] [--all-projects] [--limit] [--json] [--robot]`, `list [--project] [--all-projects] [--recent] [--json] [--robot]`, `insights [--project] [--all-projects] [--json] [--robot]`, `export <query> [--format markdown|csv] [--project] [--all-projects]`, `reindex`, `purge --before <date>`, `validate`, `status`.

The `SearchEngine` trait is the port; `storage::sqlite` is the adapter. Database is opened lazily. `Database::open_readonly()` provides read-only access for external consumers (e.g., kedral) via `SQLITE_OPEN_READ_ONLY` — fails fast if DB file missing.

### Core Pipeline

```
JSONL files → WalkDir → SHA-256 dedup → parse_sessions() ─┐
Markdown plans → discover_plan_files() → parse_plan() ─────┤
External sources → SourceRegistry::parse_all() ─────────────┤
                                                            ▼
                                              sync_files() → SQLite FTS5
                                                           → chunk_text() → embed() → sqlite-vec
                                                            │
CLI query → search() → BM25 ──────────────┐
                     → embed(query) → KNN ─┤→ RRF fusion → format_results()
```

### Hybrid Search

Search defaults to hybrid mode (BM25 + vector + RRF). Use `--lexical-only` for BM25-only. Embedding provider is configurable via `EmbeddingProvider` trait (default: all-MiniLM-L6-v2 via ONNX Runtime, download-on-first-use).

### External Source Types

Configurable in `[sources]` section of `backscroll.toml`. Source types: `ke`, `decision`, `memory`, `rule`, `spec`, `backlog`. Each has per-type parsers (whole-document or sectioned by ## headers). All filterable via `--source` flag.

### Key Design Decisions

- **Defensive parsing**: `SessionRecord` wrapper format extraction handles legacy schemas and noise.
- **Noise filtering**: Excludes `system-reminder`, `task-notification`, and subagent sessions by default.
- **External FTS5**: Uses `search_items` as content table with SQLite triggers, `snippet()` extraction, and Porter stemmer tokenizer for morphological matching.
- **Incremental sync**: SHA-256 hash per file stored in `indexed_files` table; unchanged files are skipped.
- **Plan indexing**: Markdown plans from `~/.claude/plans/` split by `##` headers, each section indexed as a separate search item with `source='plan'`.
- **Source filtering**: `search_items.source` column distinguishes sessions from plans; `--source` flag filters at query time.
- **Date filtering**: `--after`/`--before` flags filter by `search_items.timestamp` with NULL-safe guards; `--before` uses exclusive `<` comparison.
- **Multi-path config**: `session_dirs: Vec<String>` with backward-compatible `session_dir` alias and auto-discovery of `~/.claude/projects/`.
- **Auto-tagging**: Regex heuristics in `core/tagging.rs` detect session categories (debugging, refactoring, feature, testing, docs, config) during sync; stored in `session_tags` table.
- **Content-type classification**: Messages classified as `text`/`code`/`tool` based on `MessageContent::Blocks` types during sync.
- **Bundled SQLite**: `rusqlite` with `bundled` feature — no system SQLite dependency.
- **Rust edition 2024** with strict linting: clippy nursery + pedantic enabled, `-D warnings` in CI.

## Dependencies

- `clap 4.5` — CLI argument parsing with derive macros
- `rusqlite 0.39` (bundled, load_extension) — SQLite with FTS5, WAL mode, sqlite-vec support
- `sqlite-vec 0.1` — Vector similarity search extension for SQLite
- `ort =2.0.0-rc.10` — ONNX Runtime for embedding inference (load-dynamic)
- `tokenizers 0.21` — HuggingFace tokenizer for model-agnostic text encoding
- `ndarray 0.16` — N-dimensional arrays for tensor operations
- `figment 0.10` — Layered config resolution (TOML + env vars)
- `serde` / `serde_json` — Defensive JSONL deserialization with `untagged` enums
- `sha2` / `hex` — SHA-256 hashing for incremental sync deduplication
- `walkdir 2.5` — Recursive directory traversal for session discovery
- `regex 1.12` — Noise filter patterns (LazyLock compiled)
- `miette 7.6` — User-facing diagnostic error reporting
- `tracing` / `tracing-subscriber` — Structured logging with `RUST_LOG` env filter
- `insta` (dev) — Snapshot testing for output stability

## Project Documentation

- `docs/research/` — Structured research documents (hypothesize method): original feasibility study and Rust architecture pivot
- `docs/epics/` — Roadmap decomposition (E01–E12): epics, features, stories, tasks with frontmatter metadata
- `.claude/skills/backscroll/` — Claude Code skill for `/backscroll` (distributed to `~/.claude/skills/` via pre-push hook)
- Documentation is written in a mix of Spanish and English (field names like `estado`, `tipo`, `ejecutable_en` are in Spanish)

## Code Style

- `rustfmt.toml`: edition 2024, Unix newlines, `use_field_init_shorthand`, `use_try_shorthand`
- Clippy nursery + pedantic lints active (`.clippy.toml`)

## Commit Convention

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): description`).

| Type | Semver Impact | When to use |
|------|--------------|-------------|
| `feat` | minor | New user-facing functionality |
| `fix` | patch | Bug fix |
| `refactor` | none | Internal restructuring, no behavior change |
| `perf` | patch | Performance improvement |
| `test` | none | Adding or updating tests |
| `docs` | none | Documentation only |
| `chore` | none | Build, CI, dependency updates |

Breaking changes use `!` suffix (e.g., `feat!:`) for major version bumps.

### Pre-1.0 Version Strategy

While in v0.x, semver bumps follow pre-1.0 convention:

| Commit type | Bump | Example |
|---|---|---|
| `fix`, `perf`, `feat` | patch | v0.1.0 → v0.1.1 |
| `feat!`, `fix!` (breaking) | minor | v0.1.0 → v0.2.0 |

After v1.0: `feat` bumps minor, breaking bumps major (standard semver).

## Release Flow

Releases are fully automated via CI. On every push to master, CI analyzes conventional commits since the last tag, calculates the next semver version, claims it via atomic `git push origin <tag>`, injects the version into binaries at build time, builds multi-platform binaries, and creates a GitHub Release. The git tag is the source of truth for the release version — `Cargo.toml` version is only used for development. Concurrent CI runs that compute the same version lose the tag-push race and skip gracefully.

No manual release steps are needed — just push to master with conventional commit messages. Tags follow `v{VERSION}` format.

The Justfile contains only development recipes (`check`, `test`, `build`, `fmt`, `audit`). Release logic lives exclusively in CI to avoid duplication.

## CI/CD

Workflows delegate to [pablontiv/crossbeam](https://github.com/pablontiv/crossbeam) reusable workflows at `@v1`:

| Workflow | Crossbeam caller |
|---|---|
| `ci.yml` | `rust-ci.yml`, `gitleaks.yml`, `rust-release.yml` |
| `codeql.yml` | `codeql.yml` |
| `scorecard.yml` | `scorecard.yml` |

## Config Resolution Order

1. `./backscroll.toml` (current directory)
2. `~/.config/backscroll/config.toml`
3. Environment variables: `BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIRS`
4. Defaults: `~/.backscroll.db`, current directory

## Crate Path

```
backscroll (library crate) — Public API for programmatic use
backscroll::config         — Config structs (EmbeddingConfig, SourcesConfig)
backscroll::core           — Domain types and SearchEngine trait
backscroll::core::sync     — Session parsing and noise filtering
backscroll::core::plans    — Markdown plan parsing
backscroll::core::tagging  — Heuristic session auto-tagging
backscroll::core::sources  — External source parsers + SourceRegistry
backscroll::storage        — SQLite FTS5 + sqlite-vec adapter (hybrid search)

Internal modules (pub(crate)):
core::embedding            — EmbeddingProvider trait + OnnxProvider + MockProvider
core::chunking             — Text chunking pipeline
core::hybrid               — RRF fusion logic

Binary-only modules:
output                     — Output formatting
```
