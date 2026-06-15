# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: PolyForm NC](https://img.shields.io/badge/License-PolyForm%20NC-blue.svg)](LICENSE)

A **full-text search engine** for AI assistant sessions — Claude Code, Pi, and any source with an input manifest.

Backscroll treats your local AI sessions as a searchable archive: it indexes conversation logs incrementally, strips machine-generated noise, and provides instant full-text search with relevance ranking.

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

Detects your platform (Linux x86_64 / macOS aarch64), installs the binary to `~/.local/bin/`, and installs the shipped Claude, Pi, and OpenCode input presets into the user input config directory without overwriting existing manifests.

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/pablontiv/backscroll/master/install.ps1 | iex
```

Installs the binary to `%LOCALAPPDATA%\backscroll\bin\`, adds it to your PATH, and installs the shipped Claude, Pi, and OpenCode input presets into `%APPDATA%\backscroll\inputs\` without overwriting existing manifests. Compatible with Windows PowerShell 5.1+.

### Install input presets

Backscroll ships Claude, Pi, and OpenCode input presets at `inputs/claude.inputs.toml`, `inputs/pi.inputs.toml`, and `inputs/opencode.inputs.toml`. The install scripts copy those files into the user input config directory and skip existing manifests by default; set `BACKSCROLL_FORCE_INPUTS=1` only when you intentionally want to replace edited presets.
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
cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml inputs/opencode.inputs.toml "$config_dir/backscroll/inputs/"
backscroll inputs validate
backscroll inputs list
```

```powershell
$configDir = if ($env:BACKSCROLL_CONFIG_DIR) { $env:BACKSCROLL_CONFIG_DIR } else { $env:APPDATA }
$inputsDir = Join-Path $configDir "backscroll\inputs"
New-Item -ItemType Directory -Force $inputsDir | Out-Null
foreach ($name in "claude.inputs.toml", "pi.inputs.toml", "opencode.inputs.toml") {
  $dest = Join-Path $inputsDir $name
  if (-not (Test-Path $dest)) { Copy-Item (Join-Path "inputs" $name) $dest }
}
backscroll inputs validate
backscroll inputs list
```

### From Source

```bash
go install github.com/pablontiv/backscroll/cmd/backscroll@latest
```

---

## Quick Start

```bash
# 1. Confirm global input manifests are installed and valid
backscroll validate
backscroll config

# 2. Search — find past conversations by keyword (auto-syncs)
backscroll search "migration plan"

# 3. Search by project — limit results to a specific project
backscroll search "error handling" --project "backscroll"

# 4. List by input — retrieve recent Claude Code sessions
backscroll list --input claude --limit 10

# 5. Read one session — get semantic snippets from a session file
backscroll read --path ~/.claude/projects/backscroll/abc123.jsonl --tail 45 --semantic

# 6. Status — check index health
backscroll status
```

---

## Core Idea

AI assistants like Claude Code, Pi, and OpenCode produce valuable reasoning logs, but they are scattered across session files with no built-in way to search across them. Backscroll makes them **searchable**, **persistent**, and **fast**.

- Sessions are indexed incrementally — only changed files are re-processed
- Noise is stripped automatically — system-reminders, task-notifications, subagent chatter
- Search uses BM25 ranking with highlighted snippets
- Output adapts to the consumer — human-readable, JSON, or compact LLM format

Backscroll does not modify your logs. It **indexes** them.

---

## The Session Index

Each AI assistant stores conversations in its own format. Backscroll normalizes them via input manifests — shipped presets exist for Claude, Pi (both JSONL), and OpenCode (SQLite via `decode.format = "opencode"`), and any source with a compatible manifest is supported.

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
# Query commands — the core v2 surface
backscroll search <QUERY> [--input ID] [--project P] [--json] [--max-tokens N]  # Full-text search
backscroll list [--input ID] [--project P] [--order FIELD:DIR] [--limit N] [--json]  # Query by input/project
backscroll read --path <PATH> [--tail N] [--semantic]                          # Read one session file
backscroll stats --input ID [--type TYPE] [--tool TOOL] [--group-by FIELD]     # Aggregate tool-call stats

# Maintenance
backscroll status [--json]                      # Check index health and metrics
backscroll validate                             # Validate index integrity
backscroll rebuild                              # Rebuild index from source files
backscroll purge --before <DATE>                # Remove indexed items older than date
backscroll config                               # Show installed inputs and configuration
```

### Output Formats

All v2 commands produce agent-readable output by default:

```bash
# Default — tab-separated, machine-parseable
backscroll search "query terms"

# JSON — structured output for programmatic consumption
backscroll search "query terms" --json

# Pretty — human-readable formatting with highlights
backscroll search "query terms" --pretty
```

The `--fields` flag controls field density (`minimal` or `full`), and `--max-tokens` caps output by approximate token count. See [Search docs](docs/search.md) for output shapes and flag reference.

### Common workflows

**Latest Claude session with semantic snippets:**

```bash
backscroll list --input claude --project <path> --order timestamp:desc --limit 1 | head -1
PATH=$(backscroll list --input claude --project <path> --order timestamp:desc --limit 1 --json | jq -r '.path')
backscroll read --path "$PATH" --tail 45 --semantic
```

**Subagent tool-call statistics:**

```bash
backscroll stats --input pi --type tool_call --tool subagent --group-by agent --all-projects
```

### Status

`backscroll status` shows index health: files indexed, message count, projects discovered, database size, and last sync time. Auto-syncs before reporting. Use `backscroll status --json` for a versioned machine-readable status document; add `--indexed-only` to avoid auto-syncing while inspecting the current SQLite snapshot.

---

## AI-Native

Backscroll is designed as a **retrieval layer for AI assistants**. Default output is agent-readable and compact; use `--json` for structured output and `--pretty` for human formatting.

Use `--max-tokens` to fit results within a context window:

```bash
# Feed search results into an LLM pipeline (default agent-readable format)
backscroll search "architecture decisions" --max-tokens 4000

# Structured output for programmatic consumption
backscroll search "migration plan" --json --fields full | jq '.snippet'

# Project-scoped retrieval
backscroll search "error handling" --project "backscroll"
```

All output is deterministic and machine-parseable. The default format uses tab-separated values with no ANSI escape codes. Use `--pretty` for terminal formatting with highlights.

---

## Configuration

Backscroll separates application configuration from input configuration.

- **Application config** (`backscroll.toml`) controls database and embedding settings. By default, Backscroll creates an index at `~/.backscroll.db`.
- **Input config** (`*.inputs.toml`) controls what files are ingested via `backscroll search`, `backscroll list`, and `backscroll stats`. The canonical runtime location is `<config_dir>/backscroll/inputs/*.inputs.toml`, where `<config_dir>` is the OS config directory or `BACKSCROLL_CONFIG_DIR` when set.

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

Input manifests are declared as:

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

The repository presets (`inputs/*.inputs.toml`) are examples to install into the global input directory via the install script; Backscroll does not read the repository `inputs/` directory at runtime. View configured inputs with `backscroll config` or `backscroll validate`.

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

---

## Development

```bash
just check              # gofmt --check + go vet
just test               # Run all tests
just fmt                # Auto-format code (gofmt -w)
just build              # Build binary
just coverage-summary   # Go test coverage report
just audit              # go mod verify
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): description`).

---

## License

[PolyForm Noncommercial 1.0.0](LICENSE) — free for non-commercial use.
