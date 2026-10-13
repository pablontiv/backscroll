---
estado: Completed
---
# Configuration

Backscroll has three separate configuration surfaces:

- **Application config** controls Backscroll runtime settings such as the SQLite database path and embedding options.
- **Project registry** controls canonical project identity and scope through `~/.config/backscroll/projects.toml`.
- **Input config** controls ingestion. It is loaded from global, user-scoped `*.inputs.toml` manifests under `<config_dir>/backscroll/inputs/`.

`backscroll.toml` is application config only. It is not the canonical place to declare sessions, plans, Claude/Pi/OMP roots, or markdown knowledge sources.

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
| Linux | `$HOME/.config` | `$HOME/.config/backscroll/inputs/` |
| macOS | `$HOME/.config` | `$HOME/.config/backscroll/inputs/` |
| Windows | `<user-home>\.config` | `<user-home>\.config\backscroll\inputs\` |

Set non-empty `BACKSCROLL_CONFIG_DIR` to override the base directory. For example, `BACKSCROLL_CONFIG_DIR=/tmp/bs-cfg` makes Backscroll read `/tmp/bs-cfg/backscroll/inputs/*.inputs.toml`. This is an explicit cross-OS contract, not native config-directory discovery: `XDG_CONFIG_HOME`, macOS Application Support, and `%APPDATA%` are ignored for input manifests.

The repository ships source preset manifests for Claude, Pi, OMP, OpenCode, Codex, and optional Markdown inputs under `inputs/`. Backscroll reads presets only after they are copied into the user input config directory. When installing or refreshing presets, keep existing files by default so user edits are not overwritten.

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

Application files and input manifests are read once per invocation. Legacy
`[sources]` (including an empty table) is rejected from the same application
bytes used for precedence resolution. Startup discovery, ingestion, and `config`
output reuse the validated input snapshot, even if a pathname is replaced later.
There is no atomic filesystem transaction across separate configuration files.

Each manifest requires `version = 1`. Active inputs require a non-empty `id`,
at least one non-empty discovery root, and at least one non-empty include
pattern. Include and exclude patterns are syntax-checked in full, even when no
root or file exists. An omitted/empty `source` defaults to `session` (other labels
are unrestricted); an omitted/empty `decode.format` defaults to `claude`.
Unknown readers, invalid patterns, missing required fields, and malformed TOML
fail before the startup lock, index preparation, or database writes. Diagnostics
identify the manifest and offending field; definition errors also identify the
input. Inactive definitions do not undergo active-input semantic validation,
but their file must still have valid TOML and version 1. Missing discovery roots
remain tolerated. Unrecognized TOML fields are ignored, not reader extensions.
Help and version bypass configuration. Recovery still skips ordinary index
preparation and pre-handler sync; apply reuses the snapshot for post-install sync.

## Common commands

```bash
# Show the resolved application configuration and active manifests.
backscroll config
backscroll config --json

# Check prepared-index integrity; startup validates manifests separately.
backscroll validate --json

# Inspect index metrics (not build identity or the manifest inventory).
backscroll status --json

# Ordinary owners validate manifests and incrementally index changed inputs.
backscroll search --text "migration plan" --all-projects
backscroll list --order timestamp:desc --limit 20
```

Operational commands validate active manifests before startup side effects.
Owners attempt one incremental sync before executing (except recovery);
read-safe followers use the last committed snapshot, and busy `backscroll config` followers
print the validated configuration without opening the database. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, list, patterns, status, and validate.
`backscroll config --json` exposes `database`, `session_dirs`, and `inputs`; use it for
inventory, not `backscroll validate`. Check deployed version with `backscroll --version`;
for a dev binary's exact revision, inspect `go version -m "$(command -v backscroll)"`.
For recovery exceptions, backup restoration, and the separate two-snapshot
migration retention policy, see the [operator runbook](sync.md#recovery-runbook).
Use `--source-path` on search as a filter, paired with query text, for database-backed retrieval scoped to a known input path.

There is no public `inputs` or `sync` command. Manifest TOML, semantic fields, glob syntax, and reader names are validated during preflight; discovery uses that same snapshot during owner startup sync. Failures block the requested command. Use `backscroll rebuild` only after startup sync has prepared the database; the handler re-derives FTS and other derived data from the perennial database and performs no second sync. It is not a replacement for correcting manifest errors.

See [the input manifest contract](input-contract.md) for the supported fields and registered decoders. For downstream audit consumers, see the [downstream audit integration contract](audit-integration.md).

### OMP native input

`inputs/omp.inputs.toml` is an active `source = "session"` preset using the
dedicated `format = "omp"` decoder, not `pi`. It defaults to
`~/.omp/agent/sessions` with `include = ["*/*.jsonl"]`, excludes nested child
sessions by shape, and keeps `index_reasoning = false`. Installers preserve
existing OMP manifests; `BACKSCROLL_FORCE_INPUTS=1` intentionally refreshes them.
Custom `PI_CODING_AGENT_DIR`, profiles, XDG storage and `--session-dir` are not
auto-discovered: set explicit roots in a user manifest.
See the [complete OMP example](input-contract.md#complete-omp-example).

## Optional project registry

The project registry is loaded from `~/.config/backscroll/projects.toml`, independently
of `BACKSCROLL_CONFIG_DIR` (which selects application/input configuration).

```toml
[[projects]]
id = "my-app"
roots = ["/home/user/my-app"]
worktree_patterns = ["/home/user/my-app-worktrees/*"]
aliases = ["app"]
```

A missing registry is valid and yields an empty registry: project identification
retains local hints and basename fallback, and startup ingestion and `rebuild`
continue normally. A valid registry retains the existing identification order:
local hint, exact root, worktree pattern, root subpath, truncated suffix, then
basename fallback for unmatched paths.

An existing registry with invalid TOML, a directory at the file path, or a read or
permission failure is **fatal configuration**, not an empty registry. Operational
startup validates it before acquiring the startup lock or preparing/syncing the
database, including explicit `--project` and `--all-projects` invocations and
`config`. Errors identify the registry path and parser or OS cause; fix the TOML,
replace the directory with a file, or restore read permissions before retrying.
Help and version do not require registry loading. `rebuild` validates before FTS,
derived-data, or project-label changes; it must not silently relabel sessions with
fallback identities because a registry could not be read.

The Go API is `projects.LoadGlobalRegistry() (ProjectRegistry, error)`: only
`os.ErrNotExist` returns an empty registry with a nil error. Scope, ingestion, and
rebuild callers propagate all other failures before project identification.

## Project identity registry

Project identity is configured separately in `$HOME/.config/backscroll/projects.toml`
(on every host; this registry path is not changed by `BACKSCROLL_CONFIG_DIR`):

```toml
[[projects]]
id = "my-project"
roots = ["/Users/Shared/my-project"]
aliases = ["/home/shared/my-project"]
```

Roots and aliases use the same absolute, case-sensitive, cleaned path matching:
the root itself and its descendants match, but sibling prefixes do not.
Aliases explicitly authorize cross-host identity; shared path suffixes, case
folding, and truncated relative paths do not. Existing explicit
`worktree_patterns` and local `.backscroll/project.toml` hints remain supported.
If different project IDs match a path, registry attribution is withheld,
regardless of file order. Without a match, the sanitized cwd basename is used
(or `unknown` when no usable basename exists).

Migration v17 stores project evidence independently of the label. A reader's
native structured cwd attribution survives source expiry, repeated ingestion,
and rebuild without changing UUID, text, or row identity. Registry repair may
change only historical unknown or path-fallback attribution. Historical rows
default to pending unknown evidence (`NULL`); a label alone cannot prove its original source.
A completed parser pass closes unprovable retained rows as `unknown`, without
changing their label or repeatedly consuming the maintenance budget.
Available configured sources replay under the shared 200-path startup budget
to recover native proof; expired historical sources cannot acquire that proof.
Path-based fallback and registry repair exclude pending `NULL` rows, even when
the source is unavailable: absence cannot distinguish delayed replay from expiry.
Only a successful parser transaction closes pending evidence, before rebuild
repair may relabel completed unknown/path-fallback rows. Recovery preserves both
pending `NULL` and completed `unknown` evidence in the replacement index.
