---
estado: Completed
---
# Configuration

Backscroll has two separate configuration surfaces:

- **Application config** controls Backscroll runtime settings such as the SQLite database path and embedding options.
- **Input config** controls ingestion. It is loaded from global, user-scoped `*.inputs.toml` manifests under `<config_dir>/backscroll/inputs/`.

`backscroll.toml` is application config only. It is not the canonical place to declare sessions, plans, Claude/Pi roots, or markdown knowledge sources.

## Application config: `backscroll.toml`

Application config is resolved from:

| Priority | Source | Example |
|----------|--------|---------|
| 1 (highest) | `./backscroll.toml` | Project-local app config |
| 2 | `~/.config/backscroll/config.toml` | User-level app config |
| 3 | Environment variables | `BACKSCROLL_DATABASE_PATH` |
| 4 (lowest) | Built-in defaults | `~/.backscroll.db` |

App config is for global Backscroll options such as:

```toml
database_path = "/home/user/.backscroll.db"

[embedding]
model_name = "all-MiniLM-L6-v2"
similarity_threshold = 0.3
top_k = 50
rrf_k = 60
```

Historical `session_dir`, `session_dirs`, and `[sources]` keys may still appear in older configs, but they are not canonical ingestion config. Declare ingestion routes as input manifests instead.

## Input config: global `*.inputs.toml`

Canonical input manifests are loaded from exactly this runtime directory:

```text
<config_dir>/backscroll/inputs/*.inputs.toml
```

`<config_dir>` is resolved as:

| OS | Default `<config_dir>` | Manifest directory |
|---|---|---|
| Linux | `${XDG_CONFIG_HOME:-$HOME/.config}` | `${XDG_CONFIG_HOME:-$HOME/.config}/backscroll/inputs/` |
| macOS | `$HOME/Library/Application Support` | `$HOME/Library/Application Support/backscroll/inputs/` |
| Windows | `%APPDATA%` | `%APPDATA%\backscroll\inputs\` |

Set `BACKSCROLL_CONFIG_DIR` to override the base directory. For example, `BACKSCROLL_CONFIG_DIR=/tmp/bs-cfg` makes Backscroll read `/tmp/bs-cfg/backscroll/inputs/*.inputs.toml`.

The repository ships a source preset manifest at `inputs/claude.inputs.toml`. Backscroll reads source presets only after they are copied into the user input config directory. When installing or refreshing presets, keep existing files by default so user edits are not overwritten.

A minimal installed manifest looks like:

```toml
version = 1

[[inputs]]
id = "claude"
source = "session"
active = true

[inputs.discover]
roots = ["~/.claude/projects"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]

[inputs.decode]
format = "jsonl"

[inputs.map]
role = "$.message.role"

[inputs.content]
selector = "$.message.content"
```

Markdown documents use the same input list with `decode.format = "markdown"` for whole-document indexing or `decode.format = "markdown_sections"` for `## ` header splitting:

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
id = "knowledge"
source = "ke"
active = true

[inputs.discover]
roots = ["docs/knowledge"]
include = ["**/*.md"]

[inputs.decode]
format = "markdown"
```

Invalid TOML, unknown fields, unsupported versions, invalid selectors/globs/regexes, or invalid active manifests fail with an error that includes the manifest path. Missing discovery roots are skipped so shipped Claude/Pi presets can coexist on machines that only have one tool installed.

## Common commands

```bash
backscroll inputs validate
backscroll inputs list
backscroll inputs test --input claude --file ~/.claude/projects/example/session.jsonl --json
backscroll sync
```

See [the generic input contract](input-contract.md) for the full manifest schema.
