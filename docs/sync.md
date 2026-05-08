---
estado: Completed
---
# Sync & Indexing

`backscroll sync` reads session inputs (Claude and other supported sources), extracts the conversation, strips noise, and indexes everything into a local SQLite database for fast full-text search.

## CLI Usage

```bash
backscroll sync --path ~/.claude/sessions              # Index all sessions
backscroll sync --path ~/.claude/sessions --include-agents  # Include subagent sessions
```

### Flags

| Flag | Description |
|------|-------------|
| `--path <DIR>` | Directory containing session files (default: configured `session_dirs`) |
| `--include-agents` | Include subagent sessions (excluded by default) |
| `--no-plans` | Skip parsing markdown plans (`~/.claude/plans/`) |
| `--optimize` | Run FTS5 optimization after sync |

## Declarative Inputs

O01 allowed transitional ingestion through `--path`, `session_dir(s)`, and implicit Claude discovery. O02 makes TOML input manifests canonical: app settings stay in `backscroll.toml`, while session ingestion is declared in `*.inputs.toml` and/or `backscroll.inputs.d/*.toml`.

The O02 generic input manifest contract is specified in [Generic input manifest contract](input-contract.md). It describes the provider-neutral `discover -> decode -> record -> map -> content -> text -> emit` pipeline and keeps Claude/Pi conversations normalized as `source = "session"`.

Canonical manifests are loaded from:

1. `./*.inputs.toml` sorted by filename
2. `./backscroll.inputs.d/*.toml` sorted by filename

`--path` remains only as an explicit legacy CLI migration path. `session_dir`, `session_dirs`, and implicit `~/.claude/projects` discovery are non-canonical O01 compatibility mechanisms and no longer silently feed the canonical O02 session path.

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

Discovery roots may be files or directories. `~` is expanded to the user's home directory, and relative roots are resolved relative to the manifest file that declares them. Include and exclude rules are generic `globset` patterns matched against candidate paths; Backscroll does not hardcode provider-specific exclusions such as `subagents`. `follow_symlinks` defaults to `false`, so symlinked directories are not traversed unless the manifest opts in.

## Session File Format

Claude Code stores each conversation as a JSONL file — one JSON record per line. Each record has a `type` field (`user`, `assistant`, `summary`, etc.) and a `message` object containing the actual content.

Backscroll extracts only `user` and `assistant` records. Everything else — summaries, metadata, tool calls, tool results — is skipped.

## Noise Filtering

Raw session messages contain machine-generated content injected by the system. Backscroll strips all of this before indexing, producing a clean corpus of human ↔ assistant dialogue.

Filtered patterns:

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
| `Request interrupted` | Partial responses (entire message dropped) |

After filtering, if a message is empty, it is discarded entirely.

## Incremental Sync

Backscroll computes a SHA-256 hash for each session file and stores it in the database alongside the indexed content. On subsequent syncs, the hash is compared — only files whose content has changed are re-processed.

This makes repeated syncs fast: the first run indexes everything, subsequent runs skip unchanged files.

## Project Detection

Each indexed file is assigned to a project. In the legacy explicit `--path` Claude parser, resolution order is:

1. **sessions-index.json** — Claude maintains a `sessions-index.json` file mapping session UUIDs to project paths. If found, the project name is extracted from the last path component.
2. **Directory structure fallback** — If no index entry exists, the project name is inferred from the parent directory structure (the directory above `sessions/` or `subagents/`).
3. **Default** — If neither method resolves, the project is set to `"unknown"`.

O02 canonical manifests only support project values that are present in the input record via `inputs.map.project`. The MVP contract does not yet express Claude `sessions-index.json` sidecar lookup, so manifest-driven Claude sync defaults to `"unknown"` unless a future contract revision adds sidecar/project inference.

## Subagent Sessions

Claude Code spawns subagent sessions in `/subagents/` subdirectories. These are nested conversations that tend to be noisy and implementation-focused. They are excluded by default to keep the index focused on primary conversations.

Use `--include-agents` to index them alongside primary sessions.

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Sync completed successfully |
| `1` | Error (missing directory, permission denied, parse failure) |
