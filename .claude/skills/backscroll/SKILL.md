---
name: backscroll
description: "Trigger: backscroll, sesiones anteriores, where were we, ya lo hicimos, prior sessions, already done, we already did this, continue from last session. Search indexed AI session history for past decisions and prior work before answering."
user-invocable: true
allowed-tools:
  - Bash
---

# Backscroll Recipe

Backscroll is the retrieval binary for indexed AI history and declared inputs. Always run Backscroll commands before inspecting raw `session.jsonl` files.

## 1) Preflight (required)

```bash
command -v backscroll >/dev/null 2>&1
backscroll status
```

If `backscroll` is missing:

```bash
# Installer installs binary + presets into input dir
curl -fsSL https://raw.githubusercontent.com/pablontiv/backscroll/master/install.sh | bash
# Alternative: copy shipped input presets after binary is in PATH
config_dir="${BACKSCROLL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}}"
mkdir -p "$config_dir/backscroll/inputs"
cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml inputs/opencode.inputs.toml inputs/decisions.inputs.toml "$config_dir/backscroll/inputs/"
```

## 2) Canonical input location

Manifests are loaded only from:

```
<config_dir>/backscroll/inputs/*.inputs.toml
```

where `<config_dir>` is OS config directory, or `BACKSCROLL_CONFIG_DIR`.

`backscroll.toml` is app config only (DB/embedding), not the ingestion source.

## 3) Core commands

Backscroll v2 provides four canonical query commands:

| Command | Purpose |
|---|---|
| `backscroll list [--project P] [--order FIELD:DIR] [--limit N]` | List indexed items sorted and paginated |
| `backscroll search <QUERY> [--project P] [--source TYPE] [--json] [--max-tokens N]` | Full-text search with BM25 ranking |
| `backscroll read --path <PATH> [--tail N] [--semantic]` | Read one indexed session file, optionally tail and semantic rows |
| `backscroll stats --input ID [--type TYPE] [--tool TOOL] [--group-by FIELD]` | Aggregate tool-call statistics (`--input` only valid on stats) |

Maintenance commands: `status`, `validate`, `rebuild`, `purge`, `config`.

## 4) Invocation-to-command mapping

When invoked as `/skill:backscroll`:

| Invocation | Command |
|---|---|
| `/skill:backscroll` | Preflight + `backscroll status` + `backscroll list --order timestamp:desc --limit 10` |
| `/skill:backscroll QUERY` | `backscroll search "QUERY"` |
| `/skill:backscroll --recent N` | `backscroll list --order timestamp:desc --limit N` |
| `/skill:backscroll --context` | `Backscroll` context retrieval first, then optional `ref-context-mode.md` Rootline steps |

## 5) Common workflows

### 5.1) Get latest indexed session + semantic tail

```bash
backscroll list --order timestamp:desc --limit 1 --json
# Returns: {"count":1,"sessions":[{"path":"..."},...]}

# Extract path and read semantic tail:
PATH=$(backscroll list --order timestamp:desc --limit 1 --json | jq -r '.sessions[0].path')
backscroll read --path "$PATH" --tail 45 --semantic
```

**Warning — tail gap**: `--tail N` shows only the LAST N rows. Content at the start or middle of a session is invisible. If you need content from anywhere in a session, use search (see 5.6).

### 5.2) Subagent tool-call statistics

```bash
backscroll stats --input pi --type tool_call --tool subagent --group-by agent --all-projects
```

### 5.3) Search in current project

```bash
backscroll search "QUERY" --project <path>
```

### 5.4) Search across all projects

```bash
backscroll search "QUERY" --all-projects --max-tokens 4000
```

### 5.5) Read one session file

```bash
backscroll read --path /home/user/.claude/projects/<slug>/<UUID>.jsonl
```

### 5.6) Find content anywhere in a session (not just the tail)

`--tail` misses content at the start and middle of sessions. To find content at any position:

```bash
# Search across all indexed rows of all sessions
backscroll search "QUERY" --all-projects --max-tokens 8000

# Narrow to a specific session file
backscroll search "QUERY" --source-path "/path/to/session.jsonl"
```

## 6) Output formats

```bash
# Default — agent-readable
backscroll search "query"
backscroll list

# JSON
backscroll search "query" --json
backscroll list --json

# Pretty — human-readable
backscroll search "query" --pretty
```

## 7) Filter by source type

Use `--source <type>` on `search` to filter by content type. `--input` is NOT valid on `list` or `search` — only on `stats`.

```bash
backscroll search "QUERY" --source session    # only session content
backscroll search "QUERY" --source plan       # only plan content
backscroll search "QUERY" --source decision   # only decision records
```

## 8) Troubleshooting

If no results appear:

```bash
backscroll status
backscroll validate
```

`status` auto-syncs files declared by active inputs; `validate` checks index integrity. If still no results, check `backscroll config`.
