# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Backscroll is a Rust CLI tool that indexes Claude Code session JSONL logs into SQLite FTS5 for full-text search with BM25 ranking. It treats sessions as an event store with incremental sync via SHA-256 deduplication.

**Status**: Pre-1.0 — Core engine complete (sync, search, read, status). >95% test coverage. Requires Rust 1.85+.

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
│   └── sync.rs        — WalkDir, hashing, parsing JSONL with noise filtering
└── storage/
    └── sqlite.rs      — SQLite adapter (external FTS5, triggers, BM25)
```

- `main.rs` — CLI entrypoint (clap `Parser`/`Subcommand`), command dispatch, plan sync orchestration
- `config.rs` — Figment-based config resolution (TOML files + env vars), multi-path session dirs, auto-discovery of `~/.claude/projects/`
- `output.rs` — Output formatter (Text, JSON, Robot) with approximate token limiting
- `core/mod.rs` — Domain types (`SearchResult`, `ParsedFile`, `Stats`) and `SearchEngine` trait (port)
- `core/models.rs` — `SessionRecord` wrapper, `MessageContent` untagged enum for defensive parsing
- `core/plans.rs` — Markdown plan parser: splits `~/.claude/plans/*.md` by `##` headers into indexable sections
- `core/reader.rs` — Direct reading and filtering of individual session files
- `core/sync.rs` — WalkDir traversal, SHA-256 hashing, JSONL parsing, noise filter regex (LazyLock)
- `storage/sqlite.rs` — SQLite adapter (external FTS5, triggers, BM25 ranking, WAL mode, source-aware filtering)

Seven CLI commands: `sync [--path] [--include-agents] [--no-plans]`, `search <query> [--project] [--all-projects] [--json] [--robot] [--fields] [--max-tokens] [--source] [--after] [--before]`, `read <path>`, `resume <query> [--project] [--all-projects] [--robot] [--source]`, `topics [--project] [--all-projects] [--limit] [--json] [--robot]`, `list [--project] [--all-projects] [--recent] [--json] [--robot]`, `status`.

The `SearchEngine` trait is the port; `storage::sqlite` is the adapter. Database is opened lazily.

### Core Pipeline

```
JSONL files → WalkDir → SHA-256 dedup → parse_sessions() ─→ SQLite FTS5
Markdown plans → discover_plan_files() → parse_plan() ────┘       │
                                                                   │
CLI query → SearchEngine::search(source?) → BM25 ranking → format_results()
```

### Key Design Decisions

- **Defensive parsing**: `SessionRecord` wrapper format extraction handles legacy schemas and noise.
- **Noise filtering**: Excludes `system-reminder`, `task-notification`, and subagent sessions by default.
- **External FTS5**: Uses `search_items` as content table with SQLite triggers and `snippet()` extraction.
- **Incremental sync**: SHA-256 hash per file stored in `indexed_files` table; unchanged files are skipped.
- **Plan indexing**: Markdown plans from `~/.claude/plans/` split by `##` headers, each section indexed as a separate search item with `source='plan'`.
- **Source filtering**: `search_items.source` column distinguishes sessions from plans; `--source` flag filters at query time.
- **Multi-path config**: `session_dirs: Vec<String>` with backward-compatible `session_dir` alias and auto-discovery of `~/.claude/projects/`.
- **Bundled SQLite**: `rusqlite` with `bundled` feature — no system SQLite dependency.
- **Rust edition 2024** with strict linting: clippy nursery + pedantic enabled, `-D warnings` in CI.

## Dependencies

- `clap 4.5` — CLI argument parsing with derive macros
- `rusqlite 0.38` (bundled) — SQLite with FTS5, WAL mode, no system dependency
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

Releases use `just` recipes that run quality gates before tagging:

```bash
just release-patch   # check + test → bump patch → commit → tag → push
just release-minor   # check + test → bump minor → commit → tag → push
```

Version is managed via `cargo-edit` (`cargo set-version --bump`). Tags follow `v{VERSION}` format.

## Config Resolution Order

1. `./backscroll.toml` (current directory)
2. `~/.config/backscroll/config.toml`
3. Environment variables: `BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIRS`
4. Defaults: `~/.backscroll.db`, current directory

## Crate Path

```
backscroll::core        — Domain types and SearchEngine trait
backscroll::core::sync  — Session parsing and noise filtering
backscroll::core::plans — Markdown plan parsing
backscroll::storage     — SQLite FTS5 adapter
backscroll::config      — Figment configuration + auto-discovery
backscroll::output      — Output formatting
```
