# Mandatory Sync and Database-Only Retrieval

Date: 2026-08-20
Status: **APPROVED DESIGN**
Issue: [#44](https://github.com/pablontiv/backscroll/issues/44)

## Context

Backscroll is the definitive local episodic memory for coding agents. Input files are transient ingestion sources; SQLite is the perennial record that survives source expiry and supplies every user-visible retrieval.

That boundary was previously explicit and implemented. `docs/roadmap/T001-remove-public-read-command.md` identified `backscroll read` as a violation because it bypassed SQLite, and commit `f9c37eb` removed the command. The Go port later reintroduced it in `104c81e` while reconstructing the historical command inventory, then expanded it in `5080cd9` and `9d63c6f`.

A second bypass exists through `--indexed-only`. `search`, `list`, and `patterns` can skip discovery and sync and query an existing snapshot. Other commands independently choose whether to sync through `prepareIndex(..., autoSync bool)`. This makes freshness a handler-level option rather than an application invariant.

Issue #39 and PR #43 exposed the process failure. Their Cobra-backed documentation contract made living guides consistent with the registered CLI, but the CLI had already diverged from the approved North Star. Syntax consistency therefore legitimized an architectural regression.

## Goals

1. Restore SQLite as the only public retrieval source.
2. Attempt incremental startup sync exactly once before every operational command.
3. Remove every public stale-snapshot bypass.
4. Preserve one controlled recovery path after a failed mandatory startup attempt, followed by post-install sync.
5. Convert the North Star into executable CLI invariants.
6. Keep machine-readable stdout uncontaminated by sync progress or diagnostics.

## Non-goals

- No daemon, watcher, or background filesystem service.
- No public manual `sync` command.
- No direct file or UUID reader replacement.
- No alternate snapshot or stale-read flag.
- No FTS ranking, mining, or tokenizer changes.
- No rewriting of historical plans and research records.

## Locked invariants

### Database-only retrieval

Every public retrieval reads SQLite. Files discovered through active manifests are ingestion inputs only. Plans and declarative Markdown documents follow the same path as sessions:

```text
manifest -> discovery -> decode -> incremental sync -> SQLite -> query
```

`search --source-path` remains the DB-backed way to narrow retrieval to a known path, filename fragment, or session identifier embedded in `search_items.source_path`.

### Mandatory startup sync

Every operational subcommand attempts one incremental sync before its handler runs. No handler chooses freshness, and no public flag disables the attempt.

```text
Cobra parses argv
        |
        v
root startup policy
  1. load app configuration
  2. reject legacy source configuration
  3. validate every active manifest
  4. prepare a compatible index
  5. run one startup sync attempt
        |
        v
requested command handler
```

Cobra help and version output are metadata operations. They do not execute a command body, open the database, or trigger sync.

### Fail closed

Configuration, manifest, compatibility, discovery, parsing, or sync failures stop normal commands. A command never falls back to cached rows and never emits partial query output after startup failure.

### Controlled recovery

`recover` is the sole remediation continuation after startup sync fails. The sync attempt still happens first. The root policy makes the original typed diagnostic available to recovery, then allows the recovery handler to inspect and replace the active database.

A successful recovery must verify the canonical installed database and complete a post-install incremental sync before reporting success. This second attempt belongs to recovery after replacement; it does not repeat the failed startup attempt against the old database. Recovery is not a stale-read path and emits no indexed query results.

## Architecture

### Root startup policy

The existing root `PersistentPreRunE` is the single orchestration boundary. It currently rejects legacy `[sources]`; the new policy composes that check with configuration loading, manifest preflight, index compatibility, and incremental sync.

The policy must be injectable in tests so command-order behavior can be proven without relying on machine state or physical user inputs.

Responsibilities:

- identify whether a command body will execute;
- validate all ingestion configuration before database/file side effects that depend on it;
- prepare/migrate a compatible writable index;
- run incremental sync once;
- return a typed failure for normal commands;
- pass a failed-startup context to `recover` only.

It does not execute query, formatting, annotation, purge, rebuild, validation, status, or recovery business logic.

### Index preparation

`prepareIndex` no longer receives `autoSync bool`. Freshness is established before handlers enter their command-specific paths.

Read-only stale-snapshot openings remain available only as internal primitives when required for safe compatibility inspection or recovery verification. They are not selectable through a public command flag and cannot produce normal retrieval output.

Command classes may remain where they express compatibility or mutation requirements, but they must not encode whether startup sync occurs.

### Command behavior

| Command | Behavior after successful startup sync |
|---|---|
| `search` | Query the synchronized index. |
| `list` | List sessions from the synchronized index. |
| `patterns` | Compute patterns from the synchronized perennial corpus. |
| `annotate` | Write an annotation after current inputs are ingested. |
| `purge` | Apply explicit retention deletion after current inputs are ingested. |
| `rebuild` | Re-derive FTS/satellites without performing a second sync. |
| `status` | Report the post-sync state. |
| `validate` | Validate the post-sync canonical state. |
| `config` | Print effective configuration only after startup validation and sync succeed. |
| `recover` | Run normally after successful sync; after failed sync, receive the failure and continue as the sole remediation path. |

### Removed surfaces

- Remove `newReadCmd` registration and `cmd/backscroll/read.go`.
- Delete `internal/reader` after confirming it has no production consumers.
- Remove `--indexed-only` from `search`, `list`, `patterns`, `status`, and `validate`.
- Remove indexed-only parameters, help text, branching, and test helpers.
- Do not retain hidden aliases or deprecated no-op flags.

The removal is intentionally breaking. `backscroll read` becomes an unknown command and `--indexed-only` becomes an unknown flag.

## Data flow and idempotency

The mandatory sync reuses the existing SHA-256 incremental path:

1. Load active manifests.
2. Discover source references.
3. Hash each source.
4. Skip unchanged sources unless an extraction-version backfill requires reparse.
5. Parse and synchronize changed sources transactionally.
6. Re-derive bounded stale derived data as currently defined.
7. Open the prepared index for the requested handler.

Repeated command invocation is safe because unchanged inputs are hash-skipped, perennial UUID-bearing sessions use append-only identity, and derived backfills remain bounded and convergent.

The root boundary owns the single startup sync attempt. `rebuild` and normal handlers must not call sync again. `recover` alone owns a post-install sync after replacing a database whose startup attempt failed.

## Error and output contract

### Human output

- Sync progress and warnings go to stderr.
- A startup failure identifies its stage and responsible manifest/input/path.
- Normal command output begins only after successful sync.

### Machine output

- JSON and robot stdout remain valid and uncontaminated.
- Startup diagnostics preserve their machine-readable code, summary, and continuation argv.
- No partial result rows precede a startup failure.

### Recovery failure

If recovery also fails, report both the startup diagnostic and recovery failure without losing the primary cause. Do not install a partially verified database.

## Documentation contract

Living guidance expresses one operational route:

```text
active manifests -> mandatory startup sync -> perennial SQLite -> search/list/patterns
```

Required updates include:

- remove direct `backscroll read` guidance from README, `docs/read.md`, audit docs, and the shipped skill;
- remove all current `--indexed-only` guidance;
- describe `status` and `validate` as post-sync operations;
- keep `search --source-path` as indexed path lookup;
- use `decode.format = "markdown_document"` for whole-document Markdown;
- retain historical records but explicitly mark later decisions that preserved direct read or snapshot semantics as superseded by this design and ADR 0002.

The living-doc Cobra contract remains useful for syntax drift, but Cobra is not the architectural source of truth. Dedicated invariant tests constrain the command tree itself.

## Test strategy

### Root policy

- Every operational subcommand invokes startup sync exactly once.
- Startup sync completes before the handler begins.
- No handler runs after startup failure.
- Cached rows are not emitted after startup failure.
- `recover` alone can continue with the original failure context.
- A successful recovery verifies and synchronizes before success output.
- Help/version do not invoke startup sync.

### Public invariants

- Root command names exclude `read`.
- No root or child command registers `indexed-only`.
- `search --source-path` retrieves only indexed rows.
- Manifest-declared Markdown plans/documents are ingested and retrieved through SQLite.

### Output

- Human sync progress uses stderr.
- JSON/robot stdout stays parseable during success and failure.
- Startup failure produces no result payload before its diagnostic.

### Regression and gates

- Update tests that currently use `--indexed-only` as a fixture shortcut to use hermetic manifests and mandatory sync.
- Remove direct-reader tests with the deleted package.
- Update module/package layout documentation when `internal/reader` is removed.
- Run `just check`, `just test`, and `just ci`; aggregate statement coverage remains at least 85%.

## Alternatives rejected

### Per-command forced sync

Changing every `prepareIndex` call to sync would be initially smaller but preserves distributed policy. New commands could omit the call, repeating the current regression.

### Sync before Cobra execution

Running sync in `run()` before argument parsing would affect help, version, invalid arguments, output-mode detection, and recovery routing. It conflates CLI parsing with operational startup.

### Preserve direct read as a diagnostic

A direct reader still creates a second public truth and has already been mistaken for normal retrieval. Files can be inspected with ordinary filesystem tools when explicitly diagnosing ingestion; Backscroll itself remains inside the indexed-evidence boundary.

### Preserve snapshot mode

`--indexed-only` intentionally violates mandatory freshness. Reproducible downstream snapshots require a separately designed/versioned API rather than a general CLI bypass.

## Consequences

### Positive

- One enforceable freshness rule for every command.
- One public source of truth.
- New commands inherit sync automatically.
- Documentation cannot legitimize `read` merely because Cobra exposes it.
- Plans and sessions share the same ingestion/retrieval architecture.

### Negative

- Breaking removal of `read` and `--indexed-only`.
- Commands such as `config`, `status`, and `validate` now perform startup ingestion work.
- Existing audit consumers relying on snapshot mode must migrate.
- Test fixtures require broad updates because stale snapshots can no longer bypass manifests.

### Accepted operational trade-off

Startup cost is bounded by incremental hashing and unchanged-file skipping. Consistency with the episodic-memory contract takes precedence over a public stale-snapshot shortcut.

## Success criteria

The design is complete when:

1. No public retrieval bypasses SQLite.
2. No operational command handler begins before one successful sync, except the controlled recovery continuation after a failed attempt.
3. Sync failure never returns cached query output.
4. Plans are demonstrably ingested and retrieved from the perennial database.
5. CLI invariant tests fail if `read` or `--indexed-only` reappears.
6. Living documentation states the same boundary.
7. All repository gates pass at aggregate coverage >=85%.
