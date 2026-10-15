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

Backscroll ships as a **single self-contained binary**: pure Go, built with CGO disabled, so there is nothing to install alongside it. (Whether that binary ends up statically linked depends on the platform, so this makes no promise about linkage.) Runtime input manifests are separate user configuration files loaded from `<config_dir>/backscroll/inputs/*.inputs.toml`.

### Install Script (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/pablontiv/backscroll/master/install.sh | bash
```

Detects your platform (Linux x86_64 / macOS aarch64), installs the binary to `~/.local/bin/`, and installs the shipped Claude, Pi, OMP, OpenCode, and Codex input presets without overwriting existing manifests. The Unix installer uses `BACKSCROLL_CONFIG_DIR` as the config base when set and `$HOME/.config` otherwise, placing presets under `backscroll/inputs/`.

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/pablontiv/backscroll/master/install.ps1 | iex
```

Installs the binary to `%LOCALAPPDATA%\backscroll\bin\`, adds it to your PATH, and installs the shipped Claude, Pi, OMP, and Codex input presets without overwriting existing manifests. The Windows installer uses `BACKSCROLL_CONFIG_DIR` as the config base when set and `<user-home>\.config` otherwise, placing presets under `<config-base>\backscroll\inputs\`. Compatible with Windows PowerShell 5.1+.

### Install input presets

Backscroll ships Claude, Pi, OMP, OpenCode, and Codex input presets at `inputs/claude.inputs.toml`, `inputs/pi.inputs.toml`, `inputs/omp.inputs.toml`, `inputs/opencode.inputs.toml`, and `inputs/codex.inputs.toml`. Existing manifests are preserved by default; set `BACKSCROLL_FORCE_INPUTS=1` only when you intentionally want to replace edited presets. Pi subagent sessions are excluded by default and available through an explicit per-input opt-in; see the [Pi input contract](docs/input-contract.md#complete-pi-example). OMP has a dedicated native v3 reader and excludes nested child sessions by default; see the [OMP input contract](docs/input-contract.md#complete-omp-example). For Codex roots, supported records and limits, see the [Codex input contract](docs/input-contract.md#complete-codex-example).
Runtime input config directories:

| OS | Input manifest directory |
| --- | --- |
| Linux / macOS | `$HOME/.config/backscroll/inputs/` |
| Windows | `<user-home>\.config\backscroll\inputs\` |

Set `BACKSCROLL_CONFIG_DIR` to override the `<config_dir>` base. Both install scripts follow this runtime convention and otherwise use the platform defaults shown above.

If you install from a source checkout, copy presets without clobbering existing files:

```bash
config_dir="${BACKSCROLL_CONFIG_DIR:-$HOME/.config}"
mkdir -p "$config_dir/backscroll/inputs"
cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml inputs/omp.inputs.toml inputs/opencode.inputs.toml inputs/codex.inputs.toml "$config_dir/backscroll/inputs/"
backscroll validate
backscroll config
```

```powershell
$configDir = if ($env:BACKSCROLL_CONFIG_DIR) { $env:BACKSCROLL_CONFIG_DIR } else { Join-Path $HOME ".config" }
$inputsDir = Join-Path $configDir "backscroll\inputs"
New-Item -ItemType Directory -Force $inputsDir | Out-Null
foreach ($name in "claude.inputs.toml", "pi.inputs.toml", "omp.inputs.toml", "opencode.inputs.toml", "codex.inputs.toml") {
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

To install **the exact merged local commit**, rather than a published version,
use the [project-local install and build-revision verification](docs/runbooks/local-install.md).
This does not create a release or publish artifacts.

---

## Proof

```bash
# Index everything (runs automatically before any query)
backscroll status

# What did we decide about the migration?
backscroll search --text "migration plan" --all-projects

# What did that command actually return?
backscroll search --text "go test ./..." --all-projects --content-type tool

# Search supplies the selectors for exact context
backscroll search --text "$QUERY" --all-projects --json --fields minimal

# Use a non-null UUID; for uuid:null use the exact source path and ordinal
if [ "$UUID" != "null" ] && [ -n "$UUID" ]; then
  backscroll context --uuid "$UUID" --before 5 --after 5
else
  backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5
fi

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

How a file is re-synced depends on whether its messages carry identity. Sessions whose messages have a uuid — Claude Code, from schema v8 onward — sync append-only: rows already keyed by uuid are left alone and their ids stay stable. The one exception is a one-time cleanup. If the same file was indexed before v8, those older rows carry no uuid, and re-parsing would duplicate the whole file; they are deleted once, on the first re-sync after the upgrade, and never again. Sessions without one, which today is most of the corpus, are wiped and reloaded on every re-sync, so their row ids are not stable and edits to a live file replace its rows wholesale. Either way, lifecycle replacement only occurs while the source exists; retained records have no deletion path.

**Every assistant, one index.** Claude Code, Pi, OMP, OpenCode, and Codex each store sessions differently. A dedicated reader per format normalizes them behind one schema, so you search content, not file layouts. Input manifests select a registered reader and its discovery roots; adding a new provider format requires a reader implementation, not only a manifest. Message origin is likewise parser-backed: Backscroll uses native structured actor and tool boundaries, never message text or a historical stored role. An indexed message without sufficient evidence is `unknown`. Startup reparse can enrich a historical `unknown` only while its configured source remains available to its reader. Recovery can also enrich `unknown` by merging a compatible duplicate with proven origin; contradictory proven origins are rejected.

**Conversation and tool activity are indexed separately, on purpose.** Prose goes to an FTS5 index with a Porter stemmer, so "migrating" finds "migration". Tool text — commands, paths, errors — goes to a trigram index, where an exact substring like `internal/storage/sync.go` matches. An unfiltered query merges both by rank position, which is why a search never has to pick one.

**Exact context, discovery, and census are different questions.** `context` retrieves the immediate indexed neighborhood of one exact UUID or exact source-path-plus-ordinal anchor. `search` discovers ranked records from terms and filters. `patterns` computes a census: it counts commands, failures, recurring error templates, correction candidates and repeated tool sequences across the whole corpus. No individual document contains a pattern, so ranking cannot surface one.

---

## What You Can Do

### Recover what happened

Ordinary snapshot-read, metadata-read, and mutation owners validate active manifests
and attempt one incremental sync before executing. Busy readers may follow the
last committed snapshot; `recover` skips preparation/pre-sync and syncs only after
installing a replacement. See [startup classes and exceptions](docs/sync.md).
Session, plan, and Markdown files are ingestion inputs; SQLite is the perennial
record used by search, context, list, patterns, status, and validate.
Use `search` to discover relevant records. Every search result exposes its stored path, nullable UUID, and ordinal. If `uuid`/`UUID` is not null, pass that UUID to `context`. If it is null, pass the exact stored path and ordinal instead.

```bash
backscroll search --text "$QUERY" --project "$PROJECT"     # this project
backscroll search --text "$QUERY" --all-projects            # everywhere
backscroll search --text "$QUERY" --content-type tool       # commands, paths, errors
backscroll list --order timestamp:desc --limit 10             # recent sessions
backscroll search --text "$QUERY" --source-path "$SOURCE_PATH" --all-projects --json --fields minimal
if [ "$UUID" != "null" ] && [ -n "$UUID" ]; then
  backscroll context --uuid "$UUID" --before 5 --after 5 --json
else
  backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --robot
fi
```

A context UUID is opaque and is never synthesized for a result that lacks one. The alternate selector requires the exact stored source path and ordinal; duplicate rows at that coordinate fail with `context_ambiguous`. Defaults are five records on each side (maximum 50 each); record text is capped at 4000 Unicode code points. The complete successful payload defaults to a 2000-token budget (`--max-tokens`, 64–16384). See the [exact context contract](docs/context.md).

Filters worth knowing: `--after` / `--before` for a date window, `--tag` for auto-detected session categories (debugging, refactoring, testing…), `--source-path` to pin one stored input path, `--source` to keep one source class, and `--role` to filter the stored role field. The role field is not a semantic-origin guarantee.

`--lexical-only` explicitly selects BM25-only retrieval. The CLI currently has no
wired embedding provider, so ordinary search also falls back to lexical retrieval.
`--similarity-threshold F` accepts a finite value in `0..1` (default `0.3`,
`0` disables a vector threshold), but applies only to vector results; it does not
filter BM25 results or `--relax`. These flags do not enable semantic expansion.

### Discover what recurs

```bash
backscroll patterns --kind commands     # what runs most
backscroll patterns --kind failures     # what breaks, with exit codes
backscroll patterns --kind templates    # recurring error shapes
backscroll patterns --kind sequences    # workflows that repeat
backscroll patterns --kind sequences --after 2026-01-01 --before 2027-01-01 --json
backscroll patterns --kind corrections --origin human  # where you corrected course
```

`--trend` buckets commands and failures by week. `--min-support N` sets how many occurrences make a pattern. Sequences also take `--min-length` / `--max-length`.
Sequence `--after` is inclusive (`>=`) and `--before` exclusive (`<`): the example
selects timestamped events in `[2026-01-01, 2027-01-01)` before mining, excluding
events with unknown timestamps. See [sequence date filters](docs/patterns.md#sequences--frequent-workflows-exploratory).

Correction candidates are detected deterministically, never by a model, and are meant to be labelled. `--origin human|assistant|system|automation|unknown` is an opt-in corrections-only filter; it is applied before pagination, and omitting it preserves the existing population and output shape:

```bash
backscroll patterns --kind corrections --origin human --pending --batch 50 --robot
if [ -n "${UUID:-}" ] && [ "$UUID" != "null" ]; then
  backscroll annotate --uuid "$UUID" --kind correction --label "$LABEL"
else
  backscroll annotate --path "$SOURCE_PATH" --ordinal "$ORDINAL" --kind correction --label "$LABEL"
fi
```

Labelled candidates drop out of `--pending`, so the loop resumes wherever it stopped.

### Keep the index healthy

```bash
backscroll --version         # deployed version; dev builds report dev
backscroll config --json     # effective database, session_dirs, and active inputs
backscroll status            # size, counts, last sync (not build identity)
backscroll validate --json   # prepared-index integrity, not input inventory
backscroll rebuild           # re-derive search indexes from the database
```

For exact development-build provenance, use `go version -m "$(command -v backscroll)"`
and inspect `vcs.revision`/`vcs.modified`; `--version` is not a commit identifier.

For stranded indexes, first inspect `backscroll recover --from "$STRANDED_DB" --dry-run`,
then intentionally apply without `--dry-run`. The configured active database must
already exist. Recovery preserves an active backup, but a post-install sync error
leaves the replacement installed; it is **not automatic rollback**. Follow the
[recovery runbook](docs/sync.md#recovery-runbook), including backup restoration.
Automatic migration snapshots are sibling `*.snapshot-v<from>-<signature12>-to-v<to>`
files with collision suffixes and two validated snapshots retained; they are
distinct from recovery backups. See [snapshot retention](docs/sync.md#migration-snapshots-and-recovery-backups).

`rebuild` operates after the mandatory root startup sync has already prepared the database. The handler does not perform a second sync: it re-derives search indexes from stored rows, re-derives templates/correction signals/tool-event satellites where possible, and re-resolves project identities. Sessions that vanished from disk survive it untouched.

### Output for whoever is reading

Default output is human-readable text. Machine modes keep stdout parseable: human progress and warnings go to stderr, JSON/robot startup progress is discarded, and structured diagnostics remain parseable.

`--json` is available on `search`, `context`, `list`, `patterns`, `status`, `validate`, and `config`. JSON mode on context emits one complete envelope with `anchor`, `records`, `truncated`, and `omitted`. `--robot` is available on `search`, `context`, `list`, and `patterns`; context robot mode emits line-oriented anchor, envelope, and record fields. `rebuild` and `annotate` report in plain text only.

Search identity is present in every density and output mode. Text uses `Path`, `UUID`, and `Ordinal`; minimal JSON uses `source_path`, `uuid`, and `ordinal`; full JSON keeps the model casing `FilePath`, `UUID`, and `Ordinal`; robot output uses `result_N_filepath`, `result_N_uuid`, and `result_N_ordinal` for both `--fields minimal` and `--fields full`. A missing UUID is `null`, not a generated value.

On `search`, `--fields minimal|full` controls machine-output density, while `--max-tokens N` applies the truncation budget to text and robot output; JSON has no documented truncation contract. On `context`, `--max-tokens N` budgets the complete successful payload; diagnostics are exempt.

---

## Optional: Agent Integration

Backscroll is a CLI. Nothing requires an agent, and there is no MCP server — a CLI call costs a fraction of the tokens an MCP tool schema does.

Agents use the same commands with `--robot --fields minimal --max-tokens N`. The discoverable `.claude/skills/backscroll/SKILL.md` is a minimal shim: it tells the runtime to execute `backscroll --skill` and follow stdout. The complete, authoritative instructions are embedded in the binary and printed by that flag without reading configuration or modifying files. The optional [skill installer](docs/skill-installation.md) links Claude, Agents (Codex), and OpenCode to the shim in an explicitly selected stable clone. Installation requires a reviewed inventory digest; existing destinations are backed up and can be restored. Git hooks and binary installers never replace skills implicitly.

---

## Configuration

Backscroll separates application configuration from input configuration.

- **Application config** (`backscroll.toml`) controls where the database lives. By default, Backscroll creates an index at `~/.backscroll.db`.
- **Input config** (`*.inputs.toml`) controls ingestion during ordinary owner startup. The canonical runtime location is `<config_dir>/backscroll/inputs/*.inputs.toml`, where `<config_dir>` is `BACKSCROLL_CONFIG_DIR` when non-empty, otherwise `$HOME/.config` on every OS (including Windows `<user-home>\.config`). `XDG_CONFIG_HOME`, macOS Application Support, and `%APPDATA%` do not select this directory.

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

A manifest declares **where** to find inputs (`discover`) and **how** to decode them (`decode.format`). Built-in readers are `claude`, `pi`, `omp`, `opencode`, `codex`, `markdown_document` (one whole-document record), and `markdown_sections` (records split on `## ` headings, with whole-document fallback). The repository presets (`inputs/*.inputs.toml`) are examples to install into the global input directory; Backscroll does not read that repository directory at runtime. Use `backscroll config` or `backscroll config --json` for the input inventory; `validate` checks prepared-index integrity instead. See the [authoritative Markdown input contract](docs/input-contract.md#markdown-document-inputs) for manifest examples and split behavior.

See [Configuration docs](docs/configuration.md) for the full resolution order and all options.

---

## Documentation

| Topic | Description |
| ------- | ------------- |
| [Sync & Indexing](docs/sync.md) | Incremental sync, noise filtering, project detection |
| [Search Engine](docs/search.md) | BM25 ranking, output formats, token limiting |
| [Exact Context](docs/context.md) | Exact anchors, neighboring rows, output and diagnostics |
| [Pattern Discovery](docs/patterns.md) | The five censuses, the classification loop, calibration |
| [Retrieval Migration](docs/read.md) | Discovery-to-context flow over perennial SQLite |
| [Configuration](docs/configuration.md) | Config resolution, TOML format, environment variables |
| [Input Manifest Contract](docs/input-contract.md) | Supported fields and registered decoders for global `*.inputs.toml` files |
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

- the `just ci` aggregate-coverage gate when Go files change,
- the AGENTS.md / docs-update validation, and
- input preset synchronization.

Once activated, `pre-push` runs those gates and syncs input presets; `post-merge` syncs presets and documentation aggregates after a `git pull`/merge. Hooks never install the Backscroll executable or replace skill directories. Install a release with `install.sh`; released binaries then update themselves. Explicitly installed skill links follow the operator-selected stable clone; see [installation and restoration](docs/skill-installation.md).

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): description`).

---

## License

[Apache License 2.0](LICENSE) — free for commercial and non-commercial use.


### Automatic updates

Autoupdate is mandatory for valid executable release commands; there is no runtime
opt-out. It starts after command/flag/argument validation and before startup/index
work. Invalid invocations, help, version, and `--skill` never apply, fetch, or wait.
Plain `go build` produces a `dev` identity, exempt in the private updater backend.

Apply and completed fetch failures emit one concise `warning: autoupdate: ...` on
stderr without failing the command or changing stdout/JSON/robot output. Successful
updates are silent. After command execution, staging waits up to 10s; if it has not
completed, the command returns its normal result with a timeout warning.
Cancellation returns promptly without reporting a fabricated network failure;
all fetch requests are context-aware and canceled on cancellation or the staging cap,
with a 60s HTTP timeout per request. The private backend preserves the pinned picokit
Apache v1.0.0 release format while returning errors upstream swallowed; cancellation
and the cap join bounded extraction/publication cleanup before return, with unique
synced temporary files and conservative byte limits; see
[`internal/autoupdate/README.md`](internal/autoupdate/README.md).
