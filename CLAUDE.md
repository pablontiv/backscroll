# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Backscroll is a Rust CLI tool that indexes Claude Code session JSONL logs into SQLite FTS5 for full-text search with BM25 ranking. It treats sessions as an event store with incremental sync via SHA-256 deduplication.

## Commands

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

## Architecture

Ports & adapters pattern with two layers:

```
main.rs (CLI: clap)
├── config.rs          — Figment-based config (TOML files + env vars)
├── errors.rs          — thiserror + miette diagnostics
├── output.rs          — Formats output (Text, JSON, Robot) and limits tokens
├── core/
│   ├── mod.rs         — SearchResult struct, SearchEngine trait (port)
│   ├── models.rs      — SessionRecord wrapper, MessageContent untagged enum
│   ├── reader.rs      — Direct reading of single filtered session files
│   └── sync.rs        — WalkDir, hashing, parsing JSONL with noise filtering
└── storage/
    └── sqlite.rs      — SQLite adapter (external FTS5, triggers, BM25)
```

Four CLI commands: `sync [--path] [--include-agents]`, `search <query> [--project] [--json] [--robot] [--fields] [--max-tokens]`, `read <path>`, `status`.

The `SearchEngine` trait is the port; `storage::sqlite` is the adapter. Database is opened lazily.

## Key Design Decisions

- **Defensive parsing**: `SessionRecord` wrapper format extraction handles legacy schemas and noise.
- **Noise filtering**: Excludes `system-reminder`, `task-notification`, and subagent sessions by default.
- **External FTS5**: Uses `search_items` as content table with SQLite triggers and `snippet()` extraction.
- **Incremental sync**: SHA-256 hash per file stored in `indexed_files` table; unchanged files are skipped.
- **Bundled SQLite**: `rusqlite` with `bundled` feature — no system SQLite dependency.
- **Rust edition 2024** with strict linting: clippy nursery + pedantic enabled, `-D warnings` in CI.

## Code Style

- `rustfmt.toml`: edition 2024, Unix newlines, `use_field_init_shorthand`, `use_try_shorthand`
- Clippy nursery + pedantic lints active (`.clippy.toml`)
- Commits follow conventional commits (`feat:`, `fix:`, `refactor:`, `perf:`, `chore:`)
- Breaking changes use `!` suffix (e.g., `feat!:`) for major version bumps

## Testing

Unit tests are co-located in each module. Integration tests in `tests/cli.rs`. Snapshot tests use `insta`. Update snapshots with `cargo insta review`.

## Config Resolution Order

1. `./backscroll.toml` (current directory)
2. `~/.config/backscroll/config.toml`
3. Environment variables: `BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIR`
4. Defaults: `~/.backscroll.db`, current directory
