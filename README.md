# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Rust](https://img.shields.io/badge/Rust-1.94+-blue?logo=rust&logoColor=white)](https://www.rust-lang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](Cargo.toml)

A **Tier 2 search engine** for Claude Code sessions.

Backscroll treats your local AI sessions as an event store: it incrementally synchronizes JSONL logs, parses mutating schemas defensively, and provides instantaneous full-text search via SQLite FTS5.

> **Status**: Core engine complete — sync, search, and SQLite indexing functional with >95% test coverage.

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Idea](#core-idea)
- [Configuration](#configuration)
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
# 1. Sync — parse and incrementally index new sessions
backscroll sync --path ~/.claude/sessions

# 2. Search — find specific context with BM25 ranking
backscroll search "mejoras sistema tipos"

# 3. Search by project — limit results to a specific project
backscroll search "FTS5 schema" --project "backscroll"

# 4. Status — view index health and database location
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

## Development

Backscroll uses `just` as its command runner to automate quality gates and builds.

```bash
just check              # Run rustfmt and clippy
just test               # Run all unit and CLI integration tests
just coverage-summary   # Generate LLVM coverage report
just audit              # Audit supply chain for vulnerabilities
just static-build       # Build statically linked Linux binary using Zig
```

---

## License

[MIT](Cargo.toml) — free and open source.