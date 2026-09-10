---
estado: Completed
---
# Sync and Indexing

Backscroll has no public `sync` command. Ingestion is integrated into ordinary operational startup: active global input manifests are validated, changed inputs are detected by SHA-256, and only new or changed content is indexed.

Startup behavior is command-classed, not one-size-fits-all:

```text
snapshot-read: search, list, patterns, status, validate
metadata-read: config
mutation: annotate, purge, rebuild
remediation: recover
```

Snapshot-read, metadata-read, and mutation owners validate active manifests and attempt one incremental sync before executing. Remediation (`recover`) is different: it acquires and retains the mutation-grade startup lock, skips ordinary compatible-open/index preparation and pre-handler sync, and lets the recovery handler inspect or replace an index that ordinary startup might reject. Session, plan, and Markdown files are ingestion inputs; SQLite is the perennial record used by search, list, patterns, status, and validate. Use `--source-path` on search as a filter, paired with query text, for database-backed retrieval scoped to a known input path.

## Coordinated startup pipeline

```text
validated invocation
  -> classify command
  -> try <canonical-db>.startup-sync.lock
     -> snapshot-read owner: prepare/migrate -> incremental sync -> release lock -> command
     -> busy snapshot-read: compatible OpenReadOnly -> stderr warning -> query WAL snapshot
     -> metadata-read owner: prepare/migrate -> incremental sync -> release lock -> command
     -> busy metadata-read: stderr warning -> print validated config without opening the DB
     -> mutation owner: prepare/migrate -> incremental sync -> retain lock -> command
     -> remediation owner: retain mutation-grade lock -> skip ordinary compatible-open and pre-handler sync -> command
     -> busy mutation/remediation: wait <=5s -> owner or retryable sync_in_progress
```

The lock sidecar is an empty file that persists with mode `0600`; it is never deleted, and only the OS advisory lock on that file represents ownership. Startup coordination is local-host only and assumes a trusted local filesystem.

## Normal workflow

```bash
# Show the active manifests and resolved paths.
backscroll config

# Snapshot-read, metadata-read, and mutation owners perform startup sync before the handler.
backscroll search --text "migration plan"
backscroll list --order timestamp:desc --limit 20
backscroll patterns --kind templates --min-support 5
backscroll status --json
backscroll validate --json
```

Human startup sync writes progress and warnings to stderr. JSON/robot startup progress is discarded so stdout remains machine-readable, and invalid active manifests fail during preflight instead of being silently ignored. Busy followers emit `sync_in_progress` warnings to stderr; read-safe followers use the last committed WAL snapshot, config followers print validated configuration without opening the database, and mutation/remediation followers wait up to five seconds before returning a retryable failure. `backscroll recover --dry-run` reports without post-install sync; `backscroll recover` apply runs post-install sync under the same retained remediation lease before printing its report.

A search scoped to a known input path stays database-backed:

```bash
backscroll search --text "artifact literal" --source-path "*session-id*" --all-projects --json
backscroll search --text "permission denied" --source-path "*/example/*.jsonl" --all-projects --json
```

## Rebuild semantics

```bash
backscroll rebuild
```

`rebuild` is non-destructive. Mutation-class startup sync runs first and prepares the database. The rebuild handler does not perform a second sync: it re-derives both FTS5 indexes from the perennial `search_items` table, backfills derived templates/corrections/tool events from stored text where possible, and re-resolves project identities. It does not discard sessions whose files have expired.

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
format = "claude"
```

Plans and external Markdown documents are also declared as inputs. Use `decode.format = "markdown_document"` for a whole document or `decode.format = "markdown_sections"` to split on `## ` headings. See the [input manifest contract](input-contract.md) for supported fields and decoders.

## Incremental and perennial behavior

Backscroll stores a SHA-256 hash for each indexed input. Unchanged files are skipped on later startup syncs. Files with stable message UUIDs are updated append-only; legacy or UUID-less inputs retain wipe-and-reload behavior while the source exists.

Startup coordination uses owner/follower branches: snapshot-read, metadata-read, and mutation owners acquire the canonical lock and perform prepare/migrate/sync before the handler runs; a remediation owner acquires and retains the same mutation-grade lock but bypasses ordinary compatible-open and pre-handler sync; a read-safe follower validates the existing database read-only and continues on a compatible snapshot; metadata-read followers avoid opening the database; mutation and remediation followers wait up to five seconds for ownership or fail retryably with `sync_in_progress`. WAL snapshot followers remain compatible only on the same local host.

The SQLite database is the perennial event store, not a disposable cache. When a source file expires, its indexed rows remain available. Only `purge` removes retained data explicitly.

## Machine output

`--json` writes one JSON payload to stdout. JSON and robot startup progress is discarded so stdout stays parseable; human progress and warnings use stderr. Structured diagnostics remain parseable in machine modes. `--robot` output shape is command-specific: `backscroll list` and `backscroll patterns` include command-defined sections, while only `backscroll search "<query>" --robot` guarantees deterministic `result_N_field=value` lines. Search robot string values escape backslash as `\\`, carriage return as `\r`, and newline as `\n` so each field remains one line.

## Noise filtering

Each dedicated reader owns provider-specific record inclusion, text cleanup, and tool extraction. Discovery exclusions in the manifest remove configured paths such as Claude subagent sessions; readers remove provider noise such as system reminders, task notifications, and local command metadata. Pi reasoning is indexed only when its manifest sets `index_reasoning = true`.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Command and any required incremental sync completed |
| `1` | Invalid manifest, incompatible index, permission, parse, or database failure |
