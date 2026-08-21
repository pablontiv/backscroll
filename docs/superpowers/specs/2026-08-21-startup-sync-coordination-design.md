# Startup Sync Coordination and Linear Tagging

Date: 2026-08-21
Status: **APPROVED DESIGN**
Issue: [#46](https://github.com/pablontiv/backscroll/issues/46)
Related: [#47](https://github.com/pablontiv/backscroll/issues/47), [#48](https://github.com/pablontiv/backscroll/issues/48), [#49](https://github.com/pablontiv/backscroll/issues/49)

## Context

Backscroll runs one mandatory incremental startup sync before every operational command. That policy restored SQLite as the only public retrieval source, but it assumes isolated command execution. Multiple agent sessions can invoke Backscroll concurrently against the same database.

Every process currently repeats input discovery, hashing, parsing, tagging, and derived-data work before SQLite serializes writers. WAL permits concurrent readers and one writer, but it does not deduplicate work performed before the write transaction.

A changed session also triggers quadratic text aggregation in `maybeAutoSync`:

```go
var sessionText string
for _, msg := range pf.Records {
    sessionText += msg.Content + "\n"
}
```

Go strings are immutable, so every append recopies the accumulated prefix. A sampled 23.2 MiB active session spent CPU in `runtime.concatstrings`; deployed commands exceeded 120 seconds and used hundreds of MiB of transient memory.

Issue #46 requires three ordered corrections:

1. remove quadratic session-text aggregation;
2. allow at most one startup synchronizer per canonical database;
3. allow concurrent read-safe commands to use the last committed SQLite snapshot instead of repeating or waiting for sync.

This design deliberately narrows ADR 0002. Mandatory startup sync remains the default and every invocation participates in the policy, but a read-safe follower may continue against a committed snapshot when another process already owns synchronization. SQLite remains the only public retrieval source.

## Goals

1. Make tagging scale linearly with parsed message bytes without constructing a second session-sized buffer.
2. Coordinate database preparation, migration, sync, and mutating handlers across processes by canonical database path.
3. Keep read-safe commands interactive during an active sync by using a compatible read-only WAL snapshot.
4. Recover ownership automatically when the synchronizing process exits or crashes.
5. Preserve existing query stdout contracts, sync transactions, tagging results, perennial storage, and controlled recovery.
6. Prove single-flight behavior with real subprocess tests and record linear-scaling benchmark evidence.

## Non-goals

- No hashing or discovery optimization; that work belongs to issue #47.
- No startup progress UI.
- No daemon, MCP server, or shared service; issues #48 and #49 own those surfaces.
- No public stale-read flag or manual sync command.
- No change to FTS ranking, mining, tokenizers, or schema.
- No coordination across hosts or network filesystems. SQLite WAL already requires all processes to use the same host.
- No PID file, heartbeat, lease table, or lock-file deletion protocol.

## Locked Decisions

### Operating-system advisory lock

Backscroll will use `github.com/gofrs/flock` v0.13.0. The package provides nonblocking and context-bounded acquisition, uses `flock` on supported Unix-like systems, and uses `LockFileEx` on Windows.

The lock is represented by an empty sidecar adjacent to the database:

```text
<canonical-database-path>.startup-sync.lock
```

The sidecar is created with mode `0600`. It remains on disk permanently. File existence never means ownership, and Backscroll never removes the sidecar. Ownership exists only while the operating system holds the advisory lock on the open file handle.

The lock is advisory: it coordinates Backscroll processes that implement this protocol. A concurrently running older binary does not honor the sidecar and is outside the guarantee. Released installations should therefore converge on the upgraded binary before relying on single-flight behavior.

Normal paths explicitly unlock and close the handle. Unix releases the lock when all associated descriptors close. Windows releases outstanding file locks after process termination, although Microsoft documents that release timing can depend on available resources. Consequently, a command may report a bounded retryable timeout immediately after a Windows crash, but a crashed process cannot leave permanent ownership.

### Local-filesystem boundary

The protocol supports database files on a trusted local filesystem. It does not promise correct coordination over NFS, SMB, or other remote filesystems. This does not narrow Backscroll's supported deployment model because SQLite WAL itself requires all processes to be on the same host and does not work over a network filesystem.

### Command classes

Every operational command has one explicit startup class:

| Class | Commands | Busy-lock behavior |
|---|---|---|
| Read-safe snapshot | `search`, `list`, `patterns`, `status`, `validate` | Continue without a new sync after compatible read-only inspection. |
| Read-safe metadata | `config` | Continue after validated configuration/manifests; no index snapshot is required. |
| Mutation | `annotate`, `purge`, `rebuild`, `recover` | Wait up to 5 seconds, then fail retryably without running the handler. |

The two read-safe variants share nonblocking lock acquisition but differ in whether their handler consumes the index. A test enumerates the Cobra tree and rejects unclassified operational commands. Runtime fallback for an unknown class is mutation, preserving the safer behavior if registration and validation ever drift.

### Lock lifetime

The owner acquires the lock before index preparation. This coordinates first creation, compatibility migration, incremental sync, and all expensive pre-transaction work.

- A read-safe owner releases the lock after successful startup sync and before its handler opens a read-only query connection.
- A mutation owner retains the lock through the entire command handler and releases it afterward.
- `recover` retains the lock through atomic replacement, verification, and post-install sync, including the controlled continuation after startup failure.

Retaining ownership through mutations prevents another process from beginning startup sync while `annotate`, `purge`, `rebuild`, or `recover` is still changing the canonical index.

### Read-only followers

A read-safe process first attempts immediate exclusive acquisition. If another process owns the lock, the follower does not wait and does not perform discovery, hashing, parsing, migration, sync, or derived-data backfill.

An index-consuming follower instead:

1. opens the existing database read-only;
2. performs compatibility inspection without migration or side effects;
3. records concurrent-snapshot mode in startup context;
4. emits a warning to stderr;
5. executes the handler through a read-only database connection.

If the database is absent, incompatible, or requires migration, an index-consuming follower fails with a retryable diagnostic. It never creates or migrates the database while another owner is active.

`config` is the sole read-safe metadata command. Because its handler consumes only the already validated configuration and manifests, it does not inspect or open the database when following an active owner. It records concurrent-snapshot mode only as a startup-freshness signal and emits its distinct warning before printing configuration.

SQLite WAL defines visibility for index-consuming followers. A follower sees only committed transactions and never observes a partial sync. A read transaction retains the snapshot visible when it starts even if the owner commits later.

### Mutation followers

A mutation process that finds the lock occupied retries acquisition for at most five seconds, with an approximately 50 ms retry delay and caller-context cancellation.

- If it acquires the lock, it becomes the owner and executes preparation, sync, and its handler.
- If the deadline expires, it returns `sync_in_progress`, executes no mutation, and asks the caller to retry.

The five-second bound aligns with the existing SQLite `busy_timeout(5000)` and prevents commands from appearing indefinitely hung.

### Incremental tagging

`internal/tagging` will expose a small accumulator that accepts one message at a time and records matched categories. `maybeAutoSync` feeds each parsed message to the accumulator while it constructs `IndexedMessage` values, then stores the accumulator's final tag set.

The accumulator does not retain message content. Its additional memory is bounded by one message's normalization and the six category matches rather than total session size.

This preserves behavior. The current patterns cannot match across the newline inserted between messages, so matching each message independently and unioning categories produces the same set as matching the newline-joined session.

## Architecture

### `internal/startuplock`

A new package owns filesystem coordination and has no Cobra or SQLite dependency.

Responsibilities:

- canonicalize the database identity;
- derive and validate the sidecar path;
- create/open the sidecar with restricted permissions;
- acquire immediately or with a caller context;
- distinguish contention from I/O errors;
- release idempotently.

Canonicalization rules:

- for an existing database, resolve its absolute path and symlinks;
- for a missing database, resolve the existing parent directory and append the database basename;
- derive the sidecar only after canonicalization so aliases converge;
- reject an existing sidecar that is a symlink or not a regular file.

The database parent directory is a trusted boundary. A caller able to replace arbitrary entries in that directory can already replace the database itself; the lock protocol does not attempt to secure an attacker-controlled database directory.

### Startup coordinator

`defaultStartupPolicy` remains the sole orchestration boundary. Its pure validation stages remain outside the lock:

```text
Cobra argument validation
  -> resolve input directory
  -> reject legacy sources
  -> load config
  -> validate active manifests
  -> classify command
  -> coordinate by canonical DB path
```

After classification, coordination has three outcomes:

```text
                      +-> owner: prepare/migrate -> sync -> handler policy
try startup lock -----+
                      +-> busy snapshot read: inspect read-only -> stale warning -> handler
                      +-> busy metadata read: warning -> handler (no DB open)
                      +-> busy mutation: wait <=5s -> owner or retryable failure
```

Startup result/context carries:

- loaded configuration;
- typed startup failure, when present;
- access mode (`fresh` or `concurrent_snapshot`);
- an optional retained lock guard for mutation handlers.

A common command wrapper releases retained mutation guards with `defer`. Startup failures release any guard before returning unless `recover` is entering its controlled continuation.

### Index opening

`prepareIndex` will enforce the existing command distinction rather than opening both classes identically:

- `indexDataRead` opens the database with `storage.OpenReadOnly` and performs compatibility inspection without applying migration;
- `indexMutation` uses `storage.OpenCompatible` and may migrate.

The root owner still uses mutation preparation before sync. Query handlers use the data-read path after startup. This prevents a concurrent follower from accidentally obtaining a write-capable connection or applying schema changes.

### Diagnostics

A stable diagnostic code `sync_in_progress` distinguishes coordination from corruption or failed sync.

Successful read-safe follower:

```text
warning: sync_in_progress: startup sync active; using last committed index snapshot
```

`config` uses:

```text
warning: sync_in_progress: startup sync active; continuing without a new startup sync
```

Warnings always go to stderr. Text, JSON, and robot stdout retain their existing shapes byte-for-byte.

A mutation timeout or unusable follower snapshot exits nonzero with a retry instruction and no recovery continuation. `recover` remains the continuation only for actual compatible-index or sync failures.

### Cleanup errors

Explicit release is always attempted. Release is idempotent.

- If startup has not entered a handler, release failure is part of the startup error and the handler does not run.
- If a mutating handler has already committed and release then fails, the command exits nonzero and states that the operation may have committed before cleanup failed.
- The process exit remains the final recovery mechanism for the operating-system lock.

Errors are combined with `errors.Join` so an operational failure is not lost when cleanup also fails.

## Error Matrix

| Condition | Exit | Handler | Output |
|---|---:|---|---|
| Owner startup and handler succeed | 0 | Runs | Existing output |
| Busy snapshot read, compatible snapshot | 0 | Runs read-only | Existing stdout + stderr warning |
| Busy snapshot read, DB absent or incompatible | nonzero | Does not run | `sync_in_progress`, retry guidance |
| Busy `config` | 0 | Runs without DB access | Existing stdout + stderr warning |
| Busy mutation, acquisition within 5 s | depends on operation | Runs after fresh sync | Existing output |
| Busy mutation, 5 s timeout | nonzero | Does not run | `sync_in_progress`, retry guidance |
| Sidecar open/validation error | nonzero | Does not run | Lock I/O/security error |
| Owner prepare/sync failure | nonzero, except controlled recovery | Only `recover` may continue | Existing typed diagnostic/continuation |
| Owner crashes | process-specific | N/A | Next process can acquire after OS release |
| Release fails after committed mutation | nonzero | Already ran | Explicit ambiguous-commit cleanup diagnostic |

## Testing Strategy

Implementation follows TDD in four waves.

### Wave 1: linear tagging

1. Equivalence tests compare accumulator output with `Tag(strings.Join(messages, "\n"))` for every category, mixed case, empty messages, and phrase boundaries.
2. Benchmarks use synthetic 1x, 2x, and 4x record/byte sets and report `ns/op`, `B/op`, and `allocs/op`.
3. The pull request records benchmark results demonstrating approximately linear growth. CI does not enforce wall-clock ratios because shared runners make timing thresholds flaky.
4. `maybeAutoSync` regression tests verify no session-sized joined string is required and tags remain unchanged.

### Wave 2: lock primitive

Unit and subprocess tests cover:

- immediate acquisition and contention;
- context timeout and cancellation;
- release and reacquisition;
- idempotent release;
- persistent `0600` sidecar;
- canonical aliases converging on one lock;
- non-regular and symlink sidecars rejected;
- owner process killed, followed by successful acquisition from another process;
- package compilation on Linux, macOS, and Windows.

The crash test allows bounded Windows release delay rather than assuming immediate unlock.

### Wave 3: startup policy

A helper mode of the Go test binary supplies process barriers and an observable sync-entry counter.

1. Process A acquires ownership and pauses before its injected sync callback.
2. Concurrent `search`/`status` followers finish without waiting, return a previously committed row, warn on stderr, and never enter sync.
3. A mutation follower uses an injected short timeout, fails without handler side effects, and emits `sync_in_progress`.
4. Releasing A allows a later process to execute normal fresh startup.
5. Killing A allows another process to acquire and sync.
6. An absent or incompatible follower snapshot is not created or migrated.
7. JSON and robot stdout remain byte-compatible.
8. The Cobra inventory test requires an explicit class for every operational subcommand.

### Wave 4: regression gates

Fresh verification must include:

```bash
just check
go test -race ./...
just test
just ci
```

When a native Windows runner is unavailable, cross-compile the lock package tests with `GOOS=windows go test -c`. Native Windows execution remains preferable because only execution verifies `LockFileEx` behavior.

Hashing and discovery are measured separately and reported to issue #47. They are not optimized in this change.

## Documentation and Package Layout

Implementation updates:

- `docs/sync.md` for owner/follower startup semantics and warnings;
- `CLAUDE.md` implemented-package list, module tree, package table, pipeline, and key decisions;
- `go.mod` and `go.sum` for the pinned lock dependency;
- ADR 0003 for the accepted architectural exception to ADR 0002.

Historical specs and plans remain unchanged.

## Alternatives Rejected

### SQLite lease table

A lease row would expose ownership through SQL but requires opening and writing the same database whose creation and migration must be coordinated. It also introduces expiration, heartbeat, clock, and owner-death semantics. It is more complex and less reversible than an OS lock.

### `O_EXCL` PID/timestamp file

A presence-based file survives crashes and requires stale-owner detection. PID reuse, platform-specific liveness checks, and races while stealing stale files make it less reliable than kernel-managed ownership.

### Wait for strict freshness on every command

This preserves ADR 0002 literally but queues interactive reads behind potentially long sync work and recreates the observed stall under concurrency.

### Depend only on SQLite WAL and `busy_timeout`

WAL coordinates database access but does not prevent duplicate discovery, hashing, parsing, tagging, and mining before the write transaction.

### Convert Backscroll to MCP

MCP 2026-07-28 is stateless and can support a shared server, but it does not define mutual exclusion, single-flight, or SQLite writer coordination. A future server still consumes this application-level primitive. Issues #48 and #49 own that work.

## Acceptance Mapping

| Issue #46 criterion | Design mechanism |
|---|---|
| No quadratic copying | Incremental tagging accumulator; no joined session buffer |
| At most one startup sync per DB | Canonical OS advisory lock acquired before preparation |
| Concurrent queries use committed index | Read-safe follower + read-only WAL connection |
| Crash leaves no permanent lock | Descriptor/handle-owned lock; persistent file is not ownership |
| Large session avoids excessive transient memory | Per-message matching; no session duplicate |
| Existing tagging/sync/perennial behavior remains | Equivalence tests and unchanged SQLite transaction boundary |
| Automated aggregation/concurrency coverage | Benchmarks, unit tests, and real subprocess integration tests |

## Online Validation References

Validated on 2026-08-21:

- [`gofrs/flock` package documentation](https://pkg.go.dev/github.com/gofrs/flock)
- [`gofrs/flock` Unix implementation](https://github.com/gofrs/flock/blob/main/flock_unix.go)
- [`gofrs/flock` Windows implementation](https://github.com/gofrs/flock/blob/main/flock_windows.go)
- [`gofrs/flock` v0.13.0 release](https://github.com/gofrs/flock/releases/tag/v0.13.0)
- [Linux `flock(2)`](https://man7.org/linux/man-pages/man2/flock.2.html)
- [Microsoft `LockFileEx`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-lockfileex)
- [SQLite isolation](https://sqlite.org/isolation.html)
- [SQLite WAL](https://www.sqlite.org/wal.html)
