---
estado: Completed
---
# Configuration

Backscroll resolves its configuration from multiple sources, merged in priority order. No configuration is required — sensible defaults are applied automatically.

## Resolution Order

Configuration is resolved top-down. Higher priority sources override lower ones:

| Priority | Source | Example |
|----------|--------|---------|
| 1 (highest) | `./backscroll.toml` | Project-local config in current directory |
| 2 | `~/.config/backscroll/config.toml` | User-level config (XDG standard) |
| 3 | Environment variables | `BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIR` |
| 4 (lowest) | Built-in defaults | `~/.backscroll.db`, current directory |

## Config Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `database_path` | string | `~/.backscroll.db` | Path to the SQLite index database |
| `session_dir` | string | `.` (current directory) | Directory to scan for session files |

## TOML Format

```toml
database_path = "/home/user/.backscroll.db"
session_dir = "/home/user/.claude/sessions"
```

An example file is provided at `backscroll.toml.example` in the repository root.

## Environment Variables

Environment variables are prefixed with `BACKSCROLL_` and use uppercase field names:

```bash
export BACKSCROLL_DATABASE_PATH="/tmp/custom.db"
export BACKSCROLL_SESSION_DIR="/path/to/sessions"
```

Environment variables override TOML file values.

## Defaults

If no configuration is found, Backscroll uses:

- **Database**: `~/.backscroll.db` (in the user's home directory)
- **Session directory**: `.` (current working directory)

This means you can run `backscroll sync` from any directory containing session files without any configuration.
