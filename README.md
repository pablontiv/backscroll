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
- [CLI](#cli)
- [Core Idea](#core-idea)
- [Configuration](#configuration)
- [Development](#development)
- [Documentation](#documentation)
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
# 1. Sync — parse and incrementally index new sessions (excludes subagents by default)
backscroll sync --path ~/.claude/sessions
backscroll sync --path ~/.claude/sessions --include-agents # Include subagent sessions

# 2. Search — find specific context with BM25 ranking and highlighted snippets
backscroll search "mejoras sistema tipos"

# 3. Search by project — limit results to a specific project
backscroll search "FTS5 schema" --project "backscroll"

# 4. Read — view a specific session file directly, with noise filtering applied
backscroll read ~/.claude/projects/abcd/sessions/session.jsonl

# 5. LLM Integration — output as JSON or compact robot format
backscroll search "arch" --json | jq .
backscroll search "arch" --robot --max-tokens 2000

# 6. Status — view index health, file counts, and database size
backscroll status
```

---

## CLI

Backscroll ships as a **single static Rust binary** with no dependencies.

```bash
backscroll sync [--path <DIR>] [--include-agents]     # Index JSONL session files
backscroll search <QUERY> [OPTIONS]                    # Full-text search with BM25 ranking
backscroll read <PATH>                                 # Read a single session with noise filtering
backscroll status                                      # Show index health and database metrics
```

### sync

Incrementally indexes JSONL session files. Computes SHA-256 hashes to skip already indexed files. Subagent sessions are excluded by default.

```bash
backscroll sync --path ~/.claude/sessions
backscroll sync --path ~/.claude/sessions --include-agents
```

### search

Full-text search with BM25 ranking and FTS5 snippet extraction.

```bash
backscroll search "query terms"
backscroll search "FTS5 schema" --project "backscroll"   # Filter by project
backscroll search "arch" --json                          # JSON lines output
backscroll search "arch" --robot --max-tokens 2000       # Compact format with token limit
backscroll search "arch" --fields full                   # All fields (default: minimal)
```

| Flag | Description |
|------|-------------|
| `--project <NAME>` | Filter results to a specific project |
| `--json` | Output as JSON lines |
| `--robot` | Output as compact tab-separated format |
| `--fields minimal\|full` | Field set to include (default: `minimal`) |
| `--max-tokens <N>` | Approximate token limit for output |

### read

Reads a single session JSONL file directly, with noise filtering applied (strips system-reminders and task-notifications).

```bash
backscroll read ~/.claude/projects/abcd/sessions/session.jsonl
```

### status

Shows index health: files indexed, message count, projects, database size, and last sync time.

```bash
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

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): description`).

---

## Documentation

| Topic | Description |
|-------|-------------|
| [Session Search CLI Research](docs/research/backscroll-session-search-cli.md) | Original feasibility study and structured investigation |
| [Rust Architecture 2026](docs/research/backscroll-rust-architecture-2026.md) | Architecture pivot from Go to Rust with risk analysis |
| [E06: Robustez Motor](docs/epics/E06-robustez-motor/) | Parser hardening, ports & adapters refactor |
| [E07: Calidad Corpus](docs/epics/E07-calidad-corpus/) | Noise filtering, subagent exclusion, project detection |
| [E08: Output LLM-Native](docs/epics/E08-output-llm-native/) | Search enrichment, output formatting, read command |
| [E09: Hardening Post-Validacion](docs/epics/E09-hardening-post-validacion/) | Regex optimization, error handling cleanup |

---

## License

[MIT](Cargo.toml) — free and open source.