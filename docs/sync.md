---
estado: Completed
---
# Sync & Indexing

`backscroll sync` reads active global input manifests, extracts records into Backscroll's normalized message model, strips configured noise, and indexes everything into a local SQLite database for search.

## CLI Usage

```bash
backscroll inputs validate
backscroll sync
```

### Flags

| Flag | Description |
|------|-------------|
| `--no-plans` | Deprecated compatibility flag; plans are indexed only when declared as inputs |
| `--optimize` | Run FTS5 optimization after sync |
| `--no-embeddings` | Skip embedding generation during sync |

## Auto-Sync on Query

Query commands automatically index new/changed files before searching, using incremental sync (SHA-256 deduplication):

```bash
# Auto-syncs before search
backscroll search "my query"
backscroll resume "topic"
backscroll list
backscroll status
```

Auto-sync is **silent**: new content is indexed without printing progress to stdout. Sync errors emit warnings to stderr and do not block the query; the system continues with the cached index.

If no database exists, auto-sync creates it during the first query. Subsequent queries are faster due to SHA-256 deduplication.

### Indexed-only Mode

For deterministic audit consumers, `--indexed-only` skips auto-sync and opens the existing database read-only:

```bash
backscroll list --indexed-only --json
backscroll status --indexed-only
```

This mode never creates or mutates the database. If no usable index exists, commands fail with a diagnostic instructing the user to run `backscroll sync` first.

**Commands supporting `--indexed-only`**:
- `backscroll list --indexed-only`
- `backscroll status --indexed-only`
- `backscroll events query --indexed-only`
- `backscroll sessions query --indexed-only`
- `backscroll sessions list --indexed-only`

`backscroll status --json` emits a versioned status document with database path, index counts, project counts, source counts, active input metadata, and diagnostics. For the complete deterministic downstream flow, see [Downstream audit integration contract](audit-integration.md).

## Declarative Inputs

Canonical ingestion is configured with user-scoped manifests:

```text
<config_dir>/backscroll/inputs/*.inputs.toml
```

`<config_dir>` is the OS config directory, or `BACKSCROLL_CONFIG_DIR` when set:

| OS | Manifest directory |
|---|---|
| Linux | `${XDG_CONFIG_HOME:-$HOME/.config}/backscroll/inputs/` |
| macOS | `$HOME/Library/Application Support/backscroll/inputs/` |
| Windows | `%APPDATA%\backscroll\inputs\` |

Backscroll does not inspect project-local input manifests at runtime. Application config (`backscroll.toml`) remains separate from input config and does not provide canonical ingestion routes.

The generic input contract is specified in [Generic input manifest contract](input-contract.md). It describes the provider-neutral `discover -> decode -> record -> map -> content -> text -> emit` pipeline and keeps Claude/Pi conversations normalized as `source = "session"`. Markdown document inputs use `decode.format = "markdown"` for whole documents or `decode.format = "markdown_sections"` to split on `## ` headers.

A session input example:

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
follow_symlinks = false

[inputs.decode]
format = "jsonl"

[inputs.map]
role = "$.message.role"

[inputs.content]
selector = "$.message.content"
```

Plans and external markdown documents are declared explicitly as inputs:

```toml
version = 1

[[inputs]]
id = "plans"
source = "plan"
active = true

[inputs.discover]
roots = ["~/.claude/plans"]
include = ["**/*.md", "**/*.markdown"]

[inputs.decode]
format = "markdown_sections"

[[inputs]]
id = "decisions"
source = "decision"
active = true

[inputs.discover]
roots = ["docs/decisions"]
include = ["**/*.md"]

[inputs.decode]
format = "markdown"
```

Discovery roots may be files or directories. `~` is expanded to the user's home directory, and relative roots are resolved relative to the manifest file that declares them. Include and exclude rules are generic `globset` patterns matched against candidate paths. `follow_symlinks` defaults to `false`, so symlinked directories are not traversed unless the manifest opts in.

Missing discovery roots are skipped. This lets the shipped Claude, Pi, and OpenCode presets all be active even when only some tools are installed on a machine.
## Session File Format

Claude and Pi presets both decode JSONL files. Each decoded record is filtered and mapped by the installed manifest, not by hardcoded provider-specific CLI flags. The shipped Claude preset keeps `user` and `assistant` records and removes Claude noise tags. The shipped Pi preset discovers active and archived Pi session roots, maps project from the session metadata `cwd`, and keeps message records whose role is `user` or `assistant` and text content blocks. The OpenCode preset instead reads from OpenCode's SQLite database (`decode.format = "opencode"`, root: `~/.local/share/opencode/opencode.db`) rather than JSONL.

## Noise Filtering

Raw session messages can contain machine-generated content injected by agents or tools. Backscroll strips noise according to `[inputs.text].remove` rules in the active manifest. The shipped Claude preset removes patterns such as:

| Pattern | Description |
|---------|-------------|
| `<system-reminder>...</system-reminder>` | Context injected by the system |
| `<task-notification>...</task-notification>` | Background task status updates |
| `<caveat>...</caveat>` | Local command caveats |
| `<local-command-stdout>...</local-command-stdout>` | Hook and command output |
| `<command-name>...</command-name>` | Command metadata tags |
| `<command-message>...</command-message>` | Command message tags |
| `<command-args>...</command-args>` | Command argument tags |
| `Caveat: ...` (line prefix) | Caveat prefix lines |
| `Base directory: ...` (line prefix) | Base directory lines |
| `Request interrupted` | Partial responses |

After filtering, if a message is empty, it is discarded entirely when `drop_empty = true`.

## Default Limits

Query commands have sensible defaults for result limits:

| Command | Default `--limit` | Purpose |
|---------|-------------------|---------|
| `backscroll search` | 20 | Most searches find what's needed in first 20 results |
| `backscroll topics` | 30 | Show top 30 concepts |
| `backscroll list` | 20 (via `--recent`) | Show 20 most recent sessions |
| `backscroll sessions query` | 100 | Return up to 100 session metadata records |

Other defaults:
- `backscroll search --similarity-threshold` defaults to 0.3 (vector similarity floor for hybrid search)

These align with the v0 Rust implementation for consistent behavior when switching tools.

## Incremental Sync

Backscroll computes a SHA-256 hash for each input file and stores it in the database alongside the indexed content. On subsequent syncs, the hash is compared and only files whose content has changed are re-processed.

This makes repeated syncs fast: the first run indexes everything, subsequent runs skip unchanged files. Auto-sync leverages this: you can run queries repeatedly without paying the cost of re-indexing unchanged files.

## Project Detection

Manifest-driven inputs can map a project value with `inputs.map.project`. For JSONL, Backscroll evaluates that selector against each line before record filters and against emitted records, so a session metadata line can set the project for message records. If the active manifest does not map a project value, indexed messages default to `"unknown"`.

The current MVP contract does not express provider-specific sidecar lookup such as Claude `sessions-index.json` project inference. If you need project-scoped search, prefer manifests whose files contain project metadata or organize sources with explicit metadata in a future-compatible way.

## Subagent Sessions

Subagent inclusion/exclusion is data in the manifest. The shipped Claude preset excludes paths matching `**/subagents/**` in `[inputs.discover].exclude`. To index a different corpus, edit the installed user manifest and validate it with:

```bash
backscroll inputs validate
backscroll inputs test --input claude --file <PATH> --json
```

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Sync completed successfully |
| `1` | Error (permission denied, invalid manifest, parse failure) |
