# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Rust](https://img.shields.io/badge/Rust-1.85+-blue?logo=rust&logoColor=white)](https://www.rust-lang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](Cargo.toml)

A **full-text search engine** for Claude Code sessions.

Backscroll treats your local AI sessions as a searchable archive: it indexes conversation logs incrementally, strips machine-generated noise, and provides instant full-text search with relevance ranking.

> **Status**: All CLI commands functional — sync, search, read, and status.

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Core Idea](#core-idea)
- [The Session Index](#the-session-index)
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

# 2. Search — find past conversations by keyword
backscroll search "migration plan"

# 3. Search by project — limit results to a specific project
backscroll search "error handling" --project "backscroll"

# 4. Read — view a single session with noise stripped
backscroll read ~/.claude/projects/abcd/sessions/session.jsonl

# 5. Status — check index health
backscroll status
```

---

## Core Idea

Claude Code produces valuable reasoning logs, but they are scattered across session files with no built-in way to search across them. Backscroll makes them **searchable**, **persistent**, and **fast**.

- Sessions are indexed incrementally — only changed files are re-processed
- Noise is stripped automatically — system-reminders, task-notifications, subagent chatter
- Search uses BM25 ranking with highlighted snippets
- Output adapts to the consumer — human-readable, JSON, or compact LLM format

Backscroll does not modify your logs. It **indexes** them.

---

## The Session Index

Claude Code stores each conversation as a JSONL file — one JSON record per line, alternating between user messages, assistant responses, and system metadata.

Backscroll reads these files and extracts the **conversation**: user and assistant messages only. Everything else — tool calls, system-reminders, task-notifications, local command output — is stripped as noise.

### Incremental sync

Backscroll computes a SHA-256 hash for each session file. On subsequent syncs, only files whose content has changed are re-processed — syncing thousands of sessions takes seconds after the initial run.

```bash
backscroll sync --path ~/.claude/sessions              # Index all sessions
backscroll sync --path ~/.claude/sessions --include-agents  # Include subagent sessions
```

Subagent sessions are excluded by default. Project assignment is resolved automatically from Claude's `sessions-index.json` or inferred from the directory structure.

See [Sync & Indexing docs](docs/sync.md) for the full list of noise patterns, project detection logic, and subagent handling.

---

## CLI

```bash
# Indexing
backscroll sync [--path <DIR>] [--include-agents]     # Index session files
backscroll status                                      # Show index health and metrics

# Retrieval
backscroll search <QUERY> [--project] [--json|--robot] [--fields] [--max-tokens]
backscroll read <PATH>                                 # Read a single session file
```

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

The `--fields` flag controls field density (`minimal` or `full`), and `--max-tokens` caps output by approximate token count. See [Search docs](docs/search.md) for output shapes and flag reference.

### Read

`backscroll read` displays a single session file with all noise stripped, showing only the human ↔ assistant dialogue. See [Read docs](docs/read.md).

### Status

`backscroll status` shows index health: files indexed, message count, projects discovered, database size, and last sync time.

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

See [Configuration docs](docs/configuration.md) for the full resolution order and all options.

---

## Documentation

| Topic | Description |
|-------|-------------|
| [Sync & Indexing](docs/sync.md) | Incremental sync, noise filtering, project detection |
| [Search Engine](docs/search.md) | BM25 ranking, output formats, token limiting |
| [Read](docs/read.md) | Direct session reading with noise filtering |
| [Configuration](docs/configuration.md) | Config resolution, TOML format, environment variables |
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
