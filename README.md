# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Rust](https://img.shields.io/badge/Rust-1.85+-blue?logo=rust&logoColor=white)](https://www.rust-lang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](Cargo.toml)

A **full-text search engine** for Claude Code sessions.

Backscroll treats your local AI sessions as a searchable archive: it indexes conversation logs incrementally, strips machine-generated noise, and provides instant full-text search with relevance ranking.

> **Status**: Core CLI commands functional — sync, search, inputs, and status.

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

Backscroll ships as a **single static binary** with no external dependencies. Runtime input manifests are separate user configuration files loaded from `<config_dir>/backscroll/inputs/*.inputs.toml`.

### Install Script (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/pablontiv/backscroll/master/install.sh | bash
```

Detects your platform (Linux x86_64 / macOS aarch64), installs the binary to `~/.local/bin/`, and installs the shipped Claude/Pi input presets into the user input config directory without overwriting existing manifests.

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/pablontiv/backscroll/master/install.ps1 | iex
```

Installs the binary to `%LOCALAPPDATA%\backscroll\bin\`, adds it to your PATH, and installs the shipped Claude/Pi input presets into `%APPDATA%\backscroll\inputs\` without overwriting existing manifests. Compatible with Windows PowerShell 5.1+.

### Install input presets

Backscroll ships Claude and Pi input presets at `inputs/claude.inputs.toml` and `inputs/pi.inputs.toml`. The install scripts copy those files into the user input config directory and skip existing manifests by default; set `BACKSCROLL_FORCE_INPUTS=1` only when you intentionally want to replace edited presets.

Default input config directories:

| OS | Input manifest directory |
|---|---|
| Linux | `${XDG_CONFIG_HOME:-$HOME/.config}/backscroll/inputs/` |
| macOS | `$HOME/Library/Application Support/backscroll/inputs/` |
| Windows | `%APPDATA%\backscroll\inputs\` |

Set `BACKSCROLL_CONFIG_DIR` to override the `<config_dir>` base; manifests are then read from `$BACKSCROLL_CONFIG_DIR/backscroll/inputs/`.

If you install from a source checkout, copy presets without clobbering existing files:

```bash
config_dir="${BACKSCROLL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}}"
mkdir -p "$config_dir/backscroll/inputs"
cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml "$config_dir/backscroll/inputs/"
backscroll inputs validate
backscroll inputs list
```

```powershell
$configDir = if ($env:BACKSCROLL_CONFIG_DIR) { $env:BACKSCROLL_CONFIG_DIR } else { $env:APPDATA }
$inputsDir = Join-Path $configDir "backscroll\inputs"
New-Item -ItemType Directory -Force $inputsDir | Out-Null
foreach ($name in "claude.inputs.toml", "pi.inputs.toml") {
  $dest = Join-Path $inputsDir $name
  if (-not (Test-Path $dest)) { Copy-Item (Join-Path "inputs" $name) $dest }
}
backscroll inputs validate
backscroll inputs list
```

### Download Binary

Download the latest pre-compiled binary from the [Releases](https://github.com/pablontiv/backscroll/releases) page:

```bash
# Linux x86_64
curl -fsSL https://github.com/pablontiv/backscroll/releases/latest/download/backscroll-linux-x86_64 -o backscroll
chmod +x backscroll && mv backscroll ~/.local/bin/

# macOS aarch64 (Apple Silicon)
curl -fsSL https://github.com/pablontiv/backscroll/releases/latest/download/backscroll-macos-aarch64 -o backscroll
chmod +x backscroll && mv backscroll ~/.local/bin/
```

```powershell
# Windows x86_64
Invoke-WebRequest https://github.com/pablontiv/backscroll/releases/latest/download/backscroll-windows-x86_64.exe -OutFile backscroll.exe
Move-Item backscroll.exe "$env:LOCALAPPDATA\backscroll\bin\"
```

### From Source

```bash
cargo install --git https://github.com/pablontiv/backscroll.git
```

---

## Quick Start

```bash
# 1. Confirm global input manifests are installed and valid
backscroll inputs validate
backscroll inputs list

# 2. Sync — index files declared in <config_dir>/backscroll/inputs/*.inputs.toml
backscroll sync

# 3. Search — find past conversations by keyword
backscroll search "migration plan"

# 4. Search by project — limit results to a specific project
backscroll search "error handling" --project "backscroll"

# 5. Path lookup — narrow search to an indexed source path
backscroll search "migration" --source-path "*/session.jsonl" --robot

# 6. Status — check index health
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
backscroll inputs validate
backscroll sync
```

Subagent handling is controlled by the active input manifest. The shipped Claude preset excludes `subagents` paths with a discovery glob, and you can edit your installed preset if you intentionally want a different corpus.

See [Sync & Indexing docs](docs/sync.md) for input manifests, noise filtering, and project metadata behavior. See [Downstream audit integration contract](docs/audit-integration.md) for deterministic indexed-only status/session/event queries.

---

## CLI

```bash
# Input config and indexing
backscroll inputs validate                             # Validate global input manifests
backscroll inputs list                                 # List loaded manifests and inputs
backscroll inputs test --input claude --file <PATH>    # Dry-run one file without writing SQLite
backscroll sync                                        # Index files declared by active inputs
backscroll status [--json]                            # Show index health and metrics

# Retrieval
backscroll search <QUERY> [--project] [--json|--robot] [--fields] [--max-tokens] [--source-path <PATH_OR_PATTERN>]
backscroll list --indexed-only --json                  # Query the existing index without auto-sync
backscroll sessions query --jsonl --all-projects       # Stream indexed records in deterministic order
backscroll events query --jsonl --indexed-only          # Stream normalized events without auto-sync
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

### Indexed path lookup

Use `backscroll search ... --source-path <PATH_OR_PATTERN>` to retrieve matching messages from an already indexed file path through SQLite. Patterns may use `*` globs, so UUID-like session filenames can be found with `--source-path '*019e0d38-c437-7565-ba11-5dd57d516744*'`. For exhaustive local tooling, use `backscroll sessions query --jsonl` to stream indexed records in deterministic `source_path, ordinal, timestamp` order without a search term. For audit tooling that needs tool calls/results and command/error metadata, use `backscroll events query --jsonl --indexed-only`. See [Path lookup docs](docs/read.md) and the [audit integration contract](docs/audit-integration.md).

### Status

`backscroll status` shows index health: files indexed, message count, projects discovered, database size, and last sync time. Use `backscroll status --json` for a versioned machine-readable status document; add `--indexed-only` to avoid auto-syncing while inspecting the current SQLite snapshot.

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

Backscroll separates application configuration from input configuration.

- **Application config** (`backscroll.toml`) controls database and embedding settings. By default, Backscroll creates an index at `~/.backscroll.db`.
- **Input config** (`*.inputs.toml`) controls what files are ingested. The canonical runtime location is `<config_dir>/backscroll/inputs/*.inputs.toml`, where `<config_dir>` is the OS config directory or `BACKSCROLL_CONFIG_DIR` when set.

Override app settings by creating `~/.config/backscroll/config.toml` or `backscroll.toml` in the current directory:

```toml
database_path = "/home/user/.backscroll.db"

[embedding]
model_name = "all-MiniLM-L6-v2"
similarity_threshold = 0.3
```

Environment variables are also supported:

```bash
export BACKSCROLL_DATABASE_PATH="/tmp/custom.db"
```

Canonical ingestion inputs live in global user-scoped manifests:

```toml
version = 1

[[inputs]]
id = "claude"
source = "session"
active = true

[inputs.discover]
roots = ["/home/user/.claude/projects"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]

[inputs.decode]
format = "jsonl"

[inputs.map]
role = "$.message.role"

[inputs.content]
selector = "$.message.content"
```

The repository presets are examples to install into the global input directory; Backscroll does not read the repository `inputs/` directory at runtime. Historical app-config ingestion keys such as `session_dir`/`session_dirs` are not canonical input config and do not silently feed sync.

See [Configuration docs](docs/configuration.md) for the full resolution order and all options.

---

## Documentation

| Topic | Description |
|-------|-------------|
| [Sync & Indexing](docs/sync.md) | Incremental sync, noise filtering, project detection |
| [Search Engine](docs/search.md) | BM25 ranking, output formats, token limiting |
| [Indexed Path Lookup](docs/read.md) | DB-backed lookup using `search_items.source_path` |
| [Configuration](docs/configuration.md) | Config resolution, TOML format, environment variables |
| [Generic Input Contract](docs/input-contract.md) | Global `*.inputs.toml` contract for provider-neutral ingestion |
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
