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

Input files are merged and applied when `session_dirs` is default (`.`) and no `--path` is provided.

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


`inputs` is also accepted as an alias of `session_inputs` for compatibility with staged examples.

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
