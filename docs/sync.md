---
estado: Completed
---
# Sync and Indexing

Backscroll has no public `sync` command. Ingestion is integrated into operational commands: active global input manifests are validated, changed inputs are detected by SHA-256, and only new or changed content is indexed.

Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, list, patterns, status, and validate.
Use search --source-path for database-backed retrieval scoped to a known input path.

## Normal workflow

```bash
# Show the active manifests and resolved paths.
backscroll config

# Commands perform startup sync before querying SQLite.
backscroll search --text "migration plan"
backscroll list --order timestamp:desc --limit 20
backscroll patterns --kind templates --min-support 5
backscroll status --json
backscroll validate --json
```

Startup sync writes progress and warnings to stderr so JSON and robot stdout remain machine-readable. Invalid active manifests fail during preflight instead of being silently ignored.

A search scoped to a known input path stays database-backed:

```bash
backscroll search --source-path "*session-id*" --all-projects --json
backscroll search --text "permission denied" --source-path "*/example/*.jsonl" --all-projects --json
```

## Rebuild semantics

```bash
backscroll rebuild
```

`rebuild` is non-destructive. It runs incremental ingestion through the normal index preparation path, re-derives both FTS5 indexes from the perennial `search_items` table, backfills derived templates/corrections/tool events from stored text where possible, and re-resolves project identities. It does not discard sessions whose files have expired.

Use `rebuild` after index-recovery work or when derived search structures need regeneration. It is not a substitute for a removed manual sync command. `backscroll purge --before <DATE>` is the explicit deletion path.

## Declarative inputs

Canonical ingestion routes are user-scoped manifests:

```text
<config_dir>/backscroll/inputs/*.inputs.toml
```

`<config_dir>` is the OS config directory, or `BACKSCROLL_CONFIG_DIR` when set. Backscroll does not load project-local input manifests. Application configuration in `backscroll.toml` is separate and does not define canonical ingestion routes.

A session input example:

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
follow_symlinks = false

[inputs.decode]
format = "jsonl"

[inputs.map]
role = "$.message.role"

[inputs.content]
selector = "$.message.content"
```

Plans and external Markdown documents are also declared as inputs. Use `decode.format = "markdown_document"` for a whole document or `decode.format = "markdown_sections"` to split on `## ` headings. See [Generic input manifest contract](input-contract.md) for the complete schema.

## Incremental and perennial behavior

Backscroll stores a SHA-256 hash for each indexed input. Unchanged files are skipped on later startup syncs. Files with stable message UUIDs are updated append-only; legacy or UUID-less inputs retain wipe-and-reload behavior while the source exists.

The SQLite database is the perennial event store, not a disposable cache. When a source file expires, its indexed rows remain available. Only `purge` removes retained data explicitly.

## Machine output

`--json` writes one JSON payload to stdout. `--robot` writes `field=value` lines to stdout for result-set commands. Robot string values escape backslash as `\\`, carriage return as `\r`, and newline as `\n`.

## Noise filtering

Text cleanup and record inclusion are defined in each manifest. The shipped presets remove provider noise such as system reminders, task notifications, local command metadata, and configured subagent paths. Empty messages are dropped when `drop_empty = true`.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Command and any required incremental sync completed |
| `1` | Invalid manifest, incompatible index, permission, parse, or database failure |
