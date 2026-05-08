---
estado: Completed
---
# Configuration

Backscroll now separates application configuration from O02 ingestion input configuration.

O01 was transitional: `--path`, `session_dir`, `session_dirs`, and implicit Claude project discovery could feed session ingestion. In O02, the canonical ingestion flow is TOML manifest driven: `backscroll.toml` is app config, while ingestion routes live in `*.inputs.toml` files and/or `backscroll.inputs.d/*.toml` manifests that follow [the generic input contract](input-contract.md).

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

Legacy `session_dir`, `session_dirs`, and source-directory keys may still deserialize for migration compatibility, but they are not the canonical O02 input source and do not silently feed the canonical sync path. Plans and external documents (`ke`, `decision`, `memory`, `rule`, `spec`, `backlog`) are declared as inputs, not `[sources]` app config.

## Input config: `*.inputs.toml` and `backscroll.inputs.d/*.toml`

Canonical O02 ingestion manifests are loaded separately from app config:

- `./*.inputs.toml` (for example `claude.inputs.toml` or `pi.inputs.toml`)
- `./backscroll.inputs.d/*.toml` sorted by filename

Every manifest uses the contract shape:

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

[inputs.discover]
roots = ["~/.claude/plans"]
include = ["**/*.md", "**/*.markdown"]

[inputs.decode]
format = "markdown_sections"

[[inputs]]
id = "knowledge"
source = "ke"

[inputs.discover]
roots = ["docs/knowledge"]
include = ["**/*.md"]

[inputs.decode]
format = "markdown"
```

Invalid TOML, unknown fields, unsupported versions, or invalid active manifests fail with an error that includes the manifest path. Full selector/filter execution belongs to later O02 tasks; T002 establishes the separate canonical manifest loader and removes silent legacy fallback from the canonical path.

## Legacy compatibility

`backscroll sync --path <dir>` remains an explicit legacy migration path for Claude sessions. Because it is explicit on the command line, it is not treated as the canonical O02 manifest flow.

`session_dir`, `session_dirs`, and implicit `~/.claude/projects` discovery were O01 compatibility mechanisms. In O02 they are documented as non-canonical and are not used as silent fallbacks for canonical session ingestion.
