# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Rust](https://img.shields.io/badge/Rust-1.85+-blue?logo=rust&logoColor=white)](https://www.rust-lang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](Cargo.toml)

A **Tier 2 search engine** for Claude Code sessions.

Backscroll treats your local AI sessions as an event store: it incrementally synchronizes JSONL logs, parses mutating schemas defensively, and provides instantaneous full-text search via SQLite FTS5.

> **Status**: Core engine complete — sync, search, and SQLite indexing functional with >95% test coverage.

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Idea](#core-idea)
- [CLI](#cli)
- [AI-Native](#ai-native)
- [Configuration](#configuration)
- [Documentation](#documentation)
- [Development](#development)
- [License](#license)

---

## Installation

Backscroll ships as a **single static binary** with no external dependencies.

### From Releases

Download the latest pre-compiled binary from the [Releases](https://github.com/pablontiv/backscroll/releases) page.

### From Source

```bash
git clone https://github.com/pablontiv/backscroll.git
cd backscroll
cargo build --release
```

---

## Quick Start

```bash
# 1. Sync — index your Claude Code sessions
backscroll sync --path ~/.claude/sessions

# 2. Search — full-text search with BM25 ranking
backscroll search "mejoras sistema tipos"

# 3. Search by project — filter to a specific project
backscroll search "FTS5 schema" --project "backscroll"

# 4. Read — view a single session with noise stripped
backscroll read ~/.claude/projects/abcd/sessions/session.jsonl

# 5. Status — check index health
backscroll status
```

---

## Core Idea

Claude Code produces valuable reasoning logs, but they are scattered across JSONL files with unstable schemas. Backscroll makes them **searchable**, **persistent**, and **fast**.

- **Defensive Ingestion**: Handles mutating Claude schemas via `serde(untagged)` to prevent crashes on unknown block types.
- **Incremental Sync**: Computes SHA-256 hashes to skip already indexed files.
- **FTS5 + BM25**: Uses native SQLite virtual tables for full-text search with relevance ranking.
- **Concurrent Persistence**: Uses SQLite WAL mode to allow multiple readers and writers without daemon overhead.

Backscroll does not modify your logs. It **indexes** them.

---

## CLI

```bash
# Indexing
backscroll sync [--path <DIR>] [--include-agents]     # Index session files into SQLite
backscroll status                                      # Show index health and metrics

# Retrieval
backscroll search <QUERY> [--project] [--json|--robot] [--fields] [--max-tokens]
backscroll read <PATH>                                 # Read a single session file
```

### Noise Filtering

Raw Claude Code sessions contain system-reminders, task-notifications, local command output, and other machine-generated noise that buries the actual conversation. Backscroll strips all of this automatically — both during `sync` (indexing) and `read` (direct viewing).

Filtered patterns include:

- `<system-reminder>` blocks — context injected by the system, not user conversation
- `<task-notification>` blocks — background task status updates
- `<caveat>`, `<local-command-stdout>`, `<command-name>` blocks — local command metadata
- `Request interrupted` messages — partial responses with no value
- Subagent sessions (`/subagents/` paths) — excluded by default, include with `--include-agents`

The result is a clean corpus of human ↔ assistant dialogue, ready for search and analysis.

### Incremental Sync

Backscroll computes a SHA-256 hash for each session file and stores it alongside the index. On subsequent syncs, only files whose hash has changed are re-processed. This makes repeated syncs fast — even over thousands of session files.

```bash
backscroll sync --path ~/.claude/sessions              # First run: indexes everything
backscroll sync --path ~/.claude/sessions              # Second run: skips unchanged files
backscroll sync --path ~/.claude/sessions --include-agents  # Include subagent sessions
```

Project assignment is resolved automatically: first from Claude's `sessions-index.json`, then from the directory structure as fallback.

### Output Formats

Search results can be consumed in three formats, depending on whether the reader is a human, a script, or an LLM:

```bash
# Human-readable (default) — terminal bold for match highlights
backscroll search "query terms"

# JSON lines — one JSON object per result, for pipelines and scripting
backscroll search "query terms" --json

# Robot — compact tab-separated format, designed for LLM consumption
backscroll search "query terms" --robot --max-tokens 2000
```

The `--fields` flag controls field density (`minimal` or `full`), and `--max-tokens` caps output by approximate token count — useful when piping into context-limited tools.

---

## AI-Native

Backscroll is designed as a **retrieval layer for AI assistants**. The `--robot` and `--json` output formats produce stable, compact results suitable for tool use and automation.

Use `--max-tokens` to fit results within a context window:

```bash
# Feed search results into an LLM pipeline
backscroll search "architecture decisions" --robot --max-tokens 4000

# Structured output for programmatic consumption
backscroll search "migration plan" --json --fields full | jq '.snippet'

# Project-scoped retrieval
backscroll search "error handling" --project "backscroll" --robot
```

All output is deterministic and machine-parseable. No ANSI escape codes in `--json` or `--robot` modes.

---

## Configuration

Backscroll resolves its configuration automatically. By default, it creates an index at `~/.backscroll.db` and searches for sessions in the current directory.

Override defaults by creating `~/.config/backscroll/config.toml` or `backscroll.toml` in the current directory:

```toml
database_path = "/home/user/.backscroll.db"
session_dir = "/home/user/.claude/sessions"
```

Environment variables are also supported:
```bash
export BACKSCROLL_DATABASE_PATH="/tmp/custom.db"
export BACKSCROLL_SESSION_DIR="/path/to/sessions"
```

---

## Documentation

| Topic | Description |
|-------|-------------|
| [Session Search Research](docs/research/backscroll-session-search-cli.md) | Feasibility study: axioms, evidence tables, capabilities matrix |
| [Rust Architecture](docs/research/backscroll-rust-architecture-2026.md) | Stack decision: why Rust over Go, risk resolution, design patterns |

---

## Development

```bash
just check              # Run rustfmt and clippy
just test               # Run all unit and CLI integration tests
just coverage-summary   # Generate LLVM coverage report
just audit              # Audit supply chain for vulnerabilities
just static-build       # Build statically linked Linux binary using Zig
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): description`).

---

## License

[MIT](Cargo.toml) — free and open source.
