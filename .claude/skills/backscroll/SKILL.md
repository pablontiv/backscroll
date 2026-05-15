---
name: backscroll
description: Use when the user asks for previous Claude, Pi, or OpenCode sessions, forgotten discussions, recurring bugs, project history, or indexed Backscroll sessions/plans/notes. Use this before reinvesting context already covered by AI sessions.
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

## 3) Invocation-to-command mapping

When invoked as `/skill:backscroll`:

| Invocation | Commands |
|---|---|
| `/skill:backscroll` | Preflight + `backscroll status` + `backscroll list --recent 10 --robot` |
| `/skill:backscroll QUERY` | Search indexed sessions; if `QUERY` matches UUID pattern, use `search --source-path '*UUID*'` |
| `/skill:backscroll --topics` | `backscroll topics --all-projects --robot` |
| `/skill:backscroll --recent N` | `backscroll list --recent N --all-projects --robot` |
| `/skill:backscroll --context` | `Backscroll` context retrieval first, then optional `ref-context-mode.md` Rootline steps |

### 3.1) UUID/session-id path lookup

If the argument looks like UUID (`xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`), use a DB-backed `source_path` lookup instead of direct file reading.

```bash
UUID='019e0d38-c437-7565-ba11-5dd57d516744'
backscroll search "$UUID" --source sessions --source-path "*$UUID*" --all-projects --robot --max-tokens 4000
```

If the UUID is only present in the filename and not in message text, retry with nearby remembered terms as the query while keeping `--source-path "*$UUID*"`.

## 4) Non-UUID search routing (deterministic)

```bash
# Session search in current project
backscroll search "QUERY" --source sessions --robot --max-tokens 4000

# Session search across all projects
backscroll search "QUERY" --source sessions --all-projects --robot --max-tokens 4000

# Plan/notes sources
backscroll search "QUERY" --source SOURCE --all-projects --robot --max-tokens 4000
# SOURCE in: plan, ke, decision, memory, rule, spec, backlog

# Prior conversation decisions (fallback when decision source is not conversational)
backscroll search "QUERY" --source decision --all-projects --robot --max-tokens 4000
# Resume only sessions
backscroll resume "QUERY" --source sessions --all-projects --robot

# Narrow retrieval to an explicit indexed file/path fragment
backscroll search "QUERY" --source-path "PATH_OR_*PATTERN*" --all-projects --robot --max-tokens 4000

# Metadata surfaces
backscroll list --recent 10 --all-projects --robot
backscroll topics --all-projects --robot
backscroll insights --all-projects --robot
backscroll export "QUERY" --all-projects --format markdown
```

## 5) Command validity (hard constraints)

- `--robot` applies to: `search`, `list`, `topics`, `insights`, `resume`.
- `--json` applies to: `search`, `list`.
- Do not add these flags to `status`, `validate`, `sync`, `reindex`, `export`.

## 6) Source and role behavior

- `--source sessions` (plural alias) maps to indexed sessions.
- `--source plans` (plural alias) maps to `source = plan`.
- For others use singular exact values: `plan`, `ke`, `decision`, `memory`, `rule`, `spec`, `backlog`.
- `--role human` is accepted as alias for `user`; other roles and `--content-type` values are exact.

## 7) “No results” and ingestion troubleshooting

Use this order:

```bash
backscroll status
backscroll validate
```

If status and validate pass but no results appear:

```bash
backscroll sync
```

Then retry the exact search. If still no results, check that the session/source files exist at the configured paths shown by `backscroll status`.
