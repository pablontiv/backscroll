# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Rust](https://img.shields.io/badge/Rust-1.94+-blue?logo=rust&logoColor=white)](https://www.rust-lang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](Cargo.toml)

A **Tier 2 Search Engine** for Claude Code Sessions (Rust Architecture 2026).

Backscroll is a high-performance search engine designed to index and search through your Claude Code session history. Built in Rust with SQLite FTS5, it offers a fast, secure, and statically compiled alternative to traditional text search.

> **Status**: Core infrastructure and search engine complete — all CLI commands functional with >95% test coverage.

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Features](#core-features)
- [Configuration](#configuration)
- [Development](#development)
- [License](#license)

---

## Installation

Backscroll ships as a **single static binary** with no external dependencies (Zero Deps), built using Zig as a cross-linker.

You can download the latest pre-compiled binary from the [Releases](https://github.com/pablontiv/backscroll/releases) page.

### From source

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

## Core Features

Backscroll is designed for **defensive ingestion** and **high-performance search**.

- **FTS5 + BM25 Engine**: Native SQLite full-text search with relevance ranking.
- **Defensive Parsing**: Robust handling of mutating Claude JSONL schemas via `serde(untagged)`.
- **Incremental Sync**: Hash-based (SHA-256) deduplication avoids re-indexing unchanged files.
- **Concurrent Persistence**: SQLite WAL mode with busy timeouts ensures safe concurrent access without a background daemon.
- **Beautiful Diagnostics**: Rich, colorful error reporting powered by `miette`.

---

## Configuration

Backscroll resolves its configuration using hierarchical discovery. By default, it uses the `~/.backscroll.db` database and searches for sessions in the current directory.

You can override this by setting environment variables or creating a `backscroll.toml` file in `~/.config/backscroll/` or the current directory:

```toml
# backscroll.toml
database_path = "/home/user/.backscroll.db"
session_dir = "/home/user/.claude/sessions"
```

Environment variables:
```bash
export BACKSCROLL_DATABASE_PATH="/tmp/custom.db"
export BACKSCROLL_SESSION_DIR="/path/to/sessions"
```

---

## Development

Backscroll uses `just` as its command runner to automate quality gates and builds.

```bash
just check              # Run rustfmt and clippy (nursery + warnings as errors)
just test               # Run all unit and CLI integration tests
just coverage-summary   # Generate LLVM coverage report (target: >85%)
just audit              # Audit supply chain for vulnerabilities and licenses
just static-build       # Build statically linked Linux binary using Zig
just release-minor      # Bump minor version, tag, and push (triggers GitHub Release)
```

The project follows the **Ports and Adapters** architecture, decoupling the core domain (`src/core`) from the storage implementation (`src/storage/sqlite.rs`) to ensure future scalability (e.g., migrating to Tantivy).

---

## License

[MIT](Cargo.toml) — free and open source.