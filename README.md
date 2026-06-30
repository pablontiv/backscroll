# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: PolyForm NC](https://img.shields.io/badge/License-PolyForm%20NC-blue.svg)](LICENSE)

A **full-text search engine** for your AI session history — one unified search layer over every coding-agent session, whatever assistant produced it.

Backscroll is the retrieval abstraction over your local agent sessions: it normalizes each assistant's session format behind a single index, strips machine-generated noise, and provides instant full-text search with relevance ranking — so you query *what happened*, not *which tool wrote it where*.

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
backscroll validate
backscroll config
```

```powershell
$configDir = if ($env:BACKSCROLL_CONFIG_DIR) { $env:BACKSCROLL_CONFIG_DIR } else { $env:APPDATA }
$inputsDir = Join-Path $configDir "backscroll\inputs"
New-Item -ItemType Directory -Force $inputsDir | Out-Null
foreach ($name in "claude.inputs.toml", "pi.inputs.toml", "opencode.inputs.toml") {
  $dest = Join-Path $inputsDir $name
  if (-not (Test-Path $dest)) { Copy-Item (Join-Path "inputs" $name) $dest }
}
backscroll validate
backscroll config
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

# 4. List recent sessions — newest first
backscroll list --order timestamp:desc --limit 10

# 4b. Search tool activity — what command ran, or what failed
backscroll search "go test ./..." --content-type tool

# 5. Read one session — get semantic snippets from a session file
backscroll read --path ~/.claude/projects/backscroll/abc123.jsonl --tail 45 --semantic

# 6. Status — check index health
backscroll status
```

---

## Core Idea

Your coding agents produce valuable reasoning logs, but each stores them in its own format, scattered across session files with no built-in way to search across them. Backscroll is the abstraction that unifies them — making every session **searchable**, **persistent**, and **fast**, regardless of which assistant produced it.

- One index across all your agents — you search content, not per-tool file formats
- Tool activity is searchable — the commands that ran, the files touched, the outputs and errors they returned
- Sessions are indexed incrementally — only changed files are re-processed
- Noise is stripped automatically — system-reminders, task-notifications, command wrappers
- Search uses BM25 ranking with highlighted snippets
- Output adapts to the consumer — human-readable, JSON, or compact LLM format

Backscroll does not modify your logs. It **indexes** them.

---

## The Session Index

Each agent stores conversations in its own format. Backscroll normalizes them behind one index via input manifests — shipped presets cover the common agent formats (JSONL and SQLite), and any source with a compatible manifest is supported.

Backscroll extracts both the **conversation** (user and assistant messages) and the **tool activity** (the serialized tool inputs — commands, file paths, args — and their outputs and errors), indexing the latter as `content_type='tool'` so you can search what an agent actually did. Genuine noise — system-reminders, task-notifications, command wrappers — is stripped.

### Incremental sync

Backscroll computes a SHA-256 hash for each session file. On subsequent syncs, only files whose content has changed are re-processed — syncing thousands of sessions takes seconds after the initial run.

```bash
backscroll validate
backscroll list
```

Subagent handling is controlled by the active input manifest. The shipped Claude preset excludes `subagents` paths with a discovery glob, and you can edit your installed preset if you intentionally want a different corpus.

See [Sync & Indexing docs](docs/sync.md) for input manifests, noise filtering, and project metadata behavior. See [Downstream audit integration contract](docs/audit-integration.md) for deterministic indexed-only status/list/search queries.

---

## CLI

```bash
# Query commands — the core v2 surface
backscroll search <QUERY> [--project P] [--source TYPE] [--content-type text|code|tool] [--json] [--max-tokens N]  # Full-text search
backscroll list [--project P] [--order FIELD:DIR] [--limit N] [--json]  # List indexed items
backscroll read --path <PATH> [--tail N] [--semantic]                          # Read one session file

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

**Latest session with semantic snippets:**

```bash
PATH=$(backscroll list --project <path> --order timestamp:desc --limit 1 --json | jq -r '.sessions[0].path')
backscroll read --path "$PATH" --tail 45 --semantic
```

**Find what a tool did, or an error from a command:**

```bash
# Tool inputs and outputs are indexed — no need to grep raw session files
backscroll search "exit code 1" --all-projects --content-type tool
backscroll search "internal/storage/sync.go" --all-projects --content-type tool
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
- **Input config** (`*.inputs.toml`) controls what files are ingested via `backscroll search` and `backscroll list`. The canonical runtime location is `<config_dir>/backscroll/inputs/*.inputs.toml`, where `<config_dir>` is the OS config directory or `BACKSCROLL_CONFIG_DIR` when set.

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
format = "claude"
```

A manifest declares only **where** to find sessions (`discover`) and **how** to decode them (`decode.format`). Each `format` is handled by a dedicated reader that knows that agent's session schema — `claude`, `pi`, and `opencode` ship built in. The repository presets (`inputs/*.inputs.toml`) are examples to install into the global input directory via the install script; Backscroll does not read the repository `inputs/` directory at runtime. View configured inputs with `backscroll config` or `backscroll validate`.

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

### Git hooks (required, one-time per clone)

The versioned hooks in `.githooks/` are **not active until you point git at them**:

```bash
git config core.hooksPath .githooks
```

Without this, git uses `.git/hooks/` (samples only) and **every push silently skips**:

- the binary rebuild + install into `$HOME/.local/bin/backscroll` (so your installed CLI stays stale vs. the pushed code),
- the `just coverage-check` gate, and
- the CLAUDE.md / docs-update validation.

Once activated, `pre-push` runs those gates and reinstalls the binary, skill, and input presets on every push; `post-merge` reinstalls after a `git pull`/merge. Verify a hook actually fired by running the command you changed from the PATH binary — `go build` reports `version dev` (the release version is injected by CI), so confirm by behavior, not the version string.

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): description`).

---

## License

[PolyForm Noncommercial 1.0.0](LICENSE) — free for non-commercial use.
