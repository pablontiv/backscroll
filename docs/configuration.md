---
estado: Completed
---
# Configuration

Backscroll resolves its configuration from multiple sources, merged in priority order. No configuration is required — sensible defaults are applied automatically.

For session parsing, declarative input manifests are also loaded after normal config values and used only when no explicit `--path` or non-default `session_dirs` is configured.

## Resolution Order

Configuration is resolved top-down. Higher priority sources override lower ones:

| Priority | Source | Example |
|----------|--------|---------|
| 1 (highest) | `./backscroll.toml` | Project-local config in current directory |
| 2 | `~/.config/backscroll/config.toml` | User-level config (XDG standard) |
| 3 | Environment variables | `BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIR` |
| 4 (lowest) | Built-in defaults | `~/.backscroll.db`, current directory |

Declarative inputs are loaded from:
- `./backscroll.inputs.toml`
- `./backscroll.inputs.d/*.toml`

Input files are merged in deterministic order: `backscroll.inputs.toml` first,
then `backscroll.inputs.d/*.toml` sorted by filename. Invalid TOML, unknown
manifest fields, or unreadable input manifest files fail configuration loading
with an actionable error instead of being silently ignored.

Input files are applied when `session_dirs` is default (`.`) and no `--path` is provided.

Path resolution order for session parsing is:
1. CLI `--path`
2. Non-default `session_dirs` in configuration
3. Active entries in `backscroll.inputs` manifests
4. Auto-discovery under `~/.claude/projects/`

## Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `database_path` | string | `~/.backscroll.db` | Path to the SQLite index database |
| `session_dir` | string | `.` (current directory) | Backward compatible alias for `session_dirs` |
| `session_dirs` | string or array | `.` (current directory) | Directories to scan for sessions |

Session input manifests (optional):

```toml
[[session_inputs]]
source = "session"
parser = "claude"
paths = ["/home/user/.claude/projects"]
# Optional reserved selector for declarative file matching.
glob = "**/*.{json,jsonl}"
include_agents = false
active = true
```

You can also store multiple files under `backscroll.inputs.d/`:

```toml
# backscroll.inputs.d/claude.toml
[[inputs]]
source = "session"
parser = "claude"
paths = ["/path/to/dir"]

# backscroll.inputs.d/pi.toml
[[inputs]]
source = "pi"
parser = "pi"
paths = ["/path/to/pi.jsonl"]
```


`inputs` is also accepted as an alias of `session_inputs` for compatibility with staged examples. Each input entry supports:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `source` | string | `session` | Semantic source emitted to ingestion; conversations use `session` |
| `parser` | string | `claude` | Native parser adapter (`claude`, `pi`) |
| `paths` | string or array | `[]` | Files or directories to inspect |
| `glob` | string | unset | Reserved declarative file selector for the input contract |
| `include_agents` | bool | `false` | Include Claude `/subagents/` sessions when true |
| `active` | bool | `true` | Disable an input without deleting it when false |

Unknown fields are rejected so generated manifests fail fast when they drift from the supported contract.

See `backscroll.toml.example` for a full sample including optional input manifests.

## TOML Format

```toml
database_path = "/home/user/.backscroll.db"
session_dirs = [
  "/home/user/.claude/sessions",
]

[[session_inputs]]
source = "session"
parser = "claude"
paths = ["/home/user/.claude/sessions"]
include_agents = false
active = true
```

## Native Claude Parser

The `claude` parser is a native input adapter over Claude JSON/JSONL session
files. It preserves the existing ingestion semantics: `user`/`assistant` records
only, `isMeta` records skipped, `tool_use`/`tool_result` blocks removed from
text, noise tags stripped, incremental hashes preserved, and projects inferred
from `sessions-index.json` or directory layout. `/subagents/` paths are excluded
unless `include_agents = true`.

The adapter emits Backscroll's internal `ParsedFile`/`ParsedMessage` boundary and
does not execute external commands or parse arbitrary non-Claude schemas.

An example file is provided at `backscroll.toml.example` in the repository root.

## Environment Variables

Environment variables are prefixed with `BACKSCROLL_` and use uppercase field names:

```bash
export BACKSCROLL_DATABASE_PATH="/tmp/custom.db"
export BACKSCROLL_SESSION_DIRS="/path/to/sessions,/path/to/other"
```

`SESSION_DIRS` supports either a single value or comma-separated list.

Environment variables override TOML file values.

## Defaults

If no session directory input is provided, no input manifests are active, and discovery is unavailable, Backscroll uses:

- **Database**: `~/.backscroll.db` (in the user's home directory)
- **Session directory**: `.` (current working directory)

If `session_dirs` is not set, it will scan `~/.claude/projects/` during command auto-sync and listing operations.
