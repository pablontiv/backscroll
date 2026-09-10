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
| ---------- | -------- | --------- |
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
| --- | --- | --- |
| Linux | `${XDG_CONFIG_HOME:-$HOME/.config}` | `${XDG_CONFIG_HOME:-$HOME/.config}/backscroll/inputs/` |
| macOS | `$HOME/Library/Application Support` | `$HOME/Library/Application Support/backscroll/inputs/` |
| Windows | `%APPDATA%` | `%APPDATA%\backscroll\inputs\` |

Set `BACKSCROLL_CONFIG_DIR` to override the base directory. For example, `BACKSCROLL_CONFIG_DIR=/tmp/bs-cfg` makes Backscroll read `/tmp/bs-cfg/backscroll/inputs/*.inputs.toml`.

The repository ships source preset manifests for Claude, Pi, OpenCode, and optional Markdown inputs under `inputs/`. Backscroll reads presets only after they are copied into the user input config directory. When installing or refreshing presets, keep existing files by default so user edits are not overwritten.

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
format = "claude"
```

Markdown documents use the same input list with `decode.format = "markdown_document"` for whole-document indexing or `decode.format = "markdown_sections"` for `##` header splitting:

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
format = "markdown_document"
```

Malformed TOML fails with an error that includes the manifest path. An active input whose `decode.format` has no registered reader fails operational startup with an actionable error. Missing discovery roots are skipped so presets can coexist on machines that only have some supported tools installed. The current TOML loader ignores unrecognized fields; those fields do not configure or extend dedicated readers.

## Common commands

```bash
# Show the resolved application configuration and active manifests.
backscroll config
backscroll config --json

# Inspect the index and manifest health.
backscroll status --json

# Operational commands validate manifests and incrementally index changed inputs.
backscroll search --text "migration plan" --all-projects
backscroll list --order timestamp:desc --limit 20
```

Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, list, patterns, status, and validate.
Use `--source-path` on search as a filter, paired with query text, for database-backed retrieval scoped to a known input path.

There is no public `inputs` or `sync` command. Manifest TOML is loaded during command preflight; reader resolution and discovery happen during mandatory startup sync, and failures block the requested command. Use `backscroll rebuild` only after startup sync has prepared the database; the handler re-derives FTS and other derived data from the perennial database and performs no second sync. It is not a replacement for correcting manifest errors.

See [the input manifest contract](input-contract.md) for the supported fields and registered decoders. For downstream audit consumers, see the [downstream audit integration contract](audit-integration.md).
