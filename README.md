# Backscroll

[![CI](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml/badge.svg)](https://github.com/pablontiv/backscroll/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

**Backscroll turns your coding-agent sessions into a permanent, searchable record of what happened.**

Every command, error and decision — searchable across every assistant, and still there after the session files expire.

Your agents write down everything they do, then throw it away. Claude Code prunes its session files after about a month; each assistant stores them in its own format, in its own directory, with no way to search across them. The work is recorded and unreachable at the same time.

Backscroll indexes those sessions into SQLite and keeps them. Ask what a command returned six weeks ago, which errors keep recurring, or where a decision was made — and get an answer whose source file no longer exists.

---

## Table of Contents

- [Installation](#installation)
- [Proof](#proof)
- [Core Concepts](#core-concepts)
- [What You Can Do](#what-you-can-do)
- [Optional: Agent Integration](#optional-agent-integration)
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

## Proof

```bash
# Index everything (runs automatically before any query)
backscroll status

# What did we decide about the migration?
backscroll search --text "migration plan" --all-projects

# What did that command actually return?
backscroll search --text "go test ./..." --all-projects --content-type tool

# Which errors keep coming back?
backscroll patterns --kind templates --min-support 5 --all-projects
```

That last one answers a question `search` cannot. Search finds what you can already name; the census counts what recurs across every session you have:

```
Found 20 templates (min_support=5):

1. [36b55122]
   Text: error: Exit code <*>
   Occurrences: 344
   Projects: [backscroll rootline picokit crossbeam dotfiles ...]
```

---

## Core Concepts

**The database is the record, not a cache.** Session files expire; indexed sessions do not. Once a file is gone from disk, sync never walks it again, so everything indexed from it stays — that is what makes the record outlive its source.

How a file is re-synced depends on whether its messages carry identity. Sessions whose messages have a uuid — Claude Code, from schema v8 onward — sync append-only: existing rows are never touched and row ids stay stable. Sessions without one, which today is most of the corpus, are wiped and reloaded on every re-sync, so their row ids are not stable and edits to a live file replace its rows wholesale. Either way the deletion only ever happens while the file still exists; `purge --before` is the only command that removes anything on your behalf.

**Every assistant, one index.** Claude Code, Pi and OpenCode each store sessions differently. A reader per format normalizes them behind one schema, so you search content, not file layouts. New formats arrive as input manifests, not as code changes at the call site.

**Conversation and tool activity are indexed separately, on purpose.** Prose goes to an FTS5 index with a Porter stemmer, so "migrating" finds "migration". Tool text — commands, paths, errors — goes to a trigram index, where an exact substring like `internal/storage/sync.go` matches. An unfiltered query merges both by rank position, which is why a search never has to pick one.

**Retrieval and discovery are different questions.** `search` retrieves what you can name. `patterns` computes a census: it counts commands, failures, recurring error templates, correction candidates and repeated tool sequences across the whole corpus. No individual document contains a pattern, so ranking cannot surface one.

---

## What You Can Do

### Recover what happened

```bash
backscroll search --text "QUERY" --project <name>     # this project
backscroll search --text "QUERY" --all-projects       # everywhere
backscroll search --text "QUERY" --content-type tool  # commands, paths, errors
backscroll list --order timestamp:desc --limit 10     # recent sessions
backscroll read --path <FILE> --tail 45 --semantic    # one session, condensed
```

Filters worth knowing: `--after` / `--before` for a date window, `--tag` for auto-detected session categories (debugging, refactoring, testing…), `--source-path` to pin one file, `--role` to keep only what you said.

### Discover what recurs

```bash
backscroll patterns --kind commands     # what runs most
backscroll patterns --kind failures     # what breaks, with exit codes
backscroll patterns --kind templates    # recurring error shapes
backscroll patterns --kind sequences    # workflows that repeat
backscroll patterns --kind corrections  # where you corrected course
```

`--trend` buckets commands and failures by week. `--min-support N` sets how many occurrences make a pattern. Sequences also take `--min-length` / `--max-length`.

Correction candidates are detected deterministically, never by a model, and are meant to be labelled:

```bash
backscroll patterns --kind corrections --pending --batch 50 --robot
backscroll annotate --uuid <UUID> --kind correction --label "<your label>"
```

Labelled candidates drop out of `--pending`, so the loop resumes wherever it stopped.

### Keep the index healthy

```bash
backscroll status            # size, counts, last sync
backscroll validate          # integrity check
backscroll rebuild           # re-derive search indexes from the database
backscroll purge --before <DATE>   # the only deletion path
```

`rebuild` does not re-read your session files as the source of truth: it rebuilds the search indexes from what is already stored, re-derives templates and correction signals, then runs an ordinary incremental sync. Sessions that vanished from disk survive it untouched.

### Output for whoever is reading

Output is tab-separated text with no ANSI escapes by default, so it pipes cleanly.

`--json` is available on `search`, `list`, `patterns`, `status` and `config`. `--robot`, which emits `field=value` lines, is available on `search`, `list` and `patterns` — the three that return result sets. `read` takes `--pretty` instead, and `validate`, `rebuild`, `purge` and `annotate` report in plain text only.

On `search`, `--fields minimal|full` controls density and `--max-tokens N` caps output for a context window.

---

## Optional: Agent Integration

Backscroll is a CLI. Nothing requires an agent, and there is no MCP server — a CLI call costs a fraction of the tokens an MCP tool schema does.

Agents use the same commands with `--robot --fields minimal --max-tokens N`. A `/backscroll` skill for Claude Code ships in `.claude/skills/backscroll/`, and the pre-push hook installs it to `~/.claude/skills/`.

---

## Configuration

Backscroll separates application configuration from input configuration.

- **Application config** (`backscroll.toml`) controls where the database lives. By default, Backscroll creates an index at `~/.backscroll.db`.
- **Input config** (`*.inputs.toml`) controls what files are ingested via `backscroll search` and `backscroll list`. The canonical runtime location is `<config_dir>/backscroll/inputs/*.inputs.toml`, where `<config_dir>` is the OS config directory or `BACKSCROLL_CONFIG_DIR` when set.

Override app settings by creating `~/.config/backscroll/config.toml` or `backscroll.toml` in the current directory:

```toml
database_path = "/home/user/.backscroll.db"
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
| [Pattern Discovery](docs/patterns.md) | The five censuses, the classification loop, calibration |
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

[Apache License 2.0](LICENSE) — free for commercial and non-commercial use.
