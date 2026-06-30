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
config_dir=”${BACKSCROLL_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}}”
mkdir -p “$config_dir/backscroll/inputs”
cp -n inputs/claude.inputs.toml inputs/pi.inputs.toml inputs/opencode.inputs.toml inputs/decisions.inputs.toml “$config_dir/backscroll/inputs/”
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
| `backscroll list [--input ID] [--project P] [--order FIELD:DIR] [--limit N]` | Query indexed items by input/project, sorted and paginated |
| `backscroll search <QUERY> [--input ID] [--project P] [--json] [--max-tokens N]` | Full-text search with BM25 ranking |
| `backscroll read --path <PATH> [--tail N] [--semantic]` | Read one indexed session file, optionally tail and semantic rows |
| `backscroll stats --input ID [--type TYPE] [--tool TOOL] [--group-by FIELD]` | Aggregate tool-call statistics by input and optional filters |

Maintenance commands: `status`, `validate`, `rebuild`, `purge`, `config`.

## 4) Invocation-to-command mapping

When invoked as `/skill:backscroll`:

| Invocation | Command |
|---|---|
| `/skill:backscroll` | Preflight + `backscroll status` + `backscroll list --input claude --limit 10` |
| `/skill:backscroll QUERY` | `backscroll search “QUERY”` |
| `/skill:backscroll --recent N` | `backscroll list --input claude --limit N` |
| `/skill:backscroll --context` | `Backscroll` context retrieval first, then optional `ref-context-mode.md` Rootline steps |

## 5) Common workflows

### 5.1) Get latest indexed item for a Claude session + semantic tail

Retrieve the most recent Claude Code session with semantic snippets:

```bash
backscroll list --input claude --project <path> --order timestamp:desc --limit 1 | head -1
# Returns: {“path”: “...”, ...}

# Extract path and read semantic tail:
PATH=$(backscroll list --input claude --project <path> --order timestamp:desc --limit 1 --json | jq -r '.path')
backscroll read --path “$PATH” --tail 45 --semantic
```

### 5.2) Subagent tool-call statistics

Query subagent tool calls across all projects:

```bash
backscroll stats --input pi --type tool_call --tool subagent --group-by agent --all-projects
```

### 5.3) Search in current project

```bash
backscroll search “QUERY” --project <path>
```

### 5.4) Search across all projects

```bash
backscroll search “QUERY” --all-projects --max-tokens 4000
```

### 5.5) Read one session file

```bash
backscroll read --path /home/user/.claude/projects/<slug>/<UUID>.jsonl
```

## 6) Output formats

Default output is agent-readable and compact:

```bash
# Default — tab-separated, agent-readable
backscroll search “query”
backscroll list --input claude

# JSON — structured output for programmatic consumption
backscroll search “query” --json
backscroll list --input claude --json

# Pretty — human-readable formatting
backscroll search “query” --pretty
```

## 7) Filter by input

Use `--input <id>` to filter by input manifest. Common inputs: `claude`, `pi`, `opencode`, `decisions`.

```bash
# Only Claude sessions
backscroll search “QUERY” --input claude

# Only Pi sessions
backscroll list --input pi
```

## 8) Troubleshooting

If no results appear:

```bash
backscroll status
backscroll validate
```

`status` will auto-sync files declared by active inputs; `validate` checks index integrity. If still no results, verify that input manifests are installed and configured correctly by checking `backscroll config`.
