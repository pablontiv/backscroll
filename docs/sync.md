---
estado: Completed
---
# Sync and Indexing

Backscroll has no public `sync` command. Ingestion is integrated into ordinary operational startup: active global input manifests are validated, changed inputs are detected by content hash or reader watermark, and only new or changed content is indexed.

Startup behavior is command-classed, not one-size-fits-all:

```text
snapshot-read: search, context, list, patterns, status, validate
metadata-read: config
mutation: annotate, rebuild
remediation: recover
```

Snapshot-read, metadata-read, and mutation owners validate active manifests and attempt one incremental sync before executing. Remediation (`recover`) is different: it acquires and retains the mutation-grade startup lock, skips ordinary compatible-open/index preparation and pre-handler sync, and lets the recovery handler inspect or replace an index that ordinary startup might reject. Session, plan, and Markdown files are ingestion inputs; SQLite is the perennial record used by search, context, list, patterns, status, and validate. Search is ranked discovery; context retrieves an exact neighborhood from SQLite once an opaque UUID or exact source-path-plus-ordinal identity exists.

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

The lock sidecar is an empty file that persists with mode `0600` on Unix; it is never deleted, and only the OS advisory lock on that file represents ownership. Startup coordination is local-host only and assumes a trusted local filesystem.

On Unix, a pre-existing sidecar must be a regular file owned by the effective user with exactly `0600` permissions and no special mode bits. Modes `0644` and `0666` fail with `unsafe startup lock sidecar` (surfaced at startup stage `sync_lock` with the existing `migration_failed` diagnostic), without chmod, deletion, database preparation, or sync; `0600` remains valid. The portable flock API cannot chmod its held descriptor, so Backscroll rejects unsafe permissions rather than repairing a pathname. Symlinks and nonregular files are rejected; POSIX opens also refuse symlink traversal and do not block on a substituted FIFO. After acquiring the OS lock, Backscroll validates the held descriptor's mode and ownership and compares its identity with the initial sidecar and the current nonsymlink path before returning a lease. A detected replacement releases the lock and fails closed. Windows retains regular-file, symlink, and path-identity validation; POSIX mode/UID checks do not apply there, and Windows ACL/owner validation is outside C13.

The parent directory must remain trusted: these checks detect acquisition-boundary replacements, not arbitrary later mutation by another process able to rename files in that directory. Do not replace or remove a sidecar while processes may be using it. To repair an unsafe mode, first stop all Backscroll processes, verify the regular file and its ownership in a trusted directory, and explicitly set its mode to `0600` before retrying.

## Snapshot and recovery durability by platform

Migration snapshots (including their ownership markers) and recovery destination/backup files are flushed before publication or replacement. File synchronization opens an existing file without creation or truncation: Windows uses a writable handle because `FlushFileBuffers` rejects read-only handles; Unix retains read-only handles, so readable files need not be writable. Open, sync, and close failures propagate, including permission errors; file bytes and permissions are not changed by synchronization.

Directory synchronization is shared by migration snapshots and recovery. On Darwin and Linux it opens and synchronizes the directory, and **all errors propagate**, including unsupported-filesystem errors. On Windows directory synchronization is unsupported and deliberately a no-op: regular-file contents are flushed, but this primitive does **not** guarantee crash durability of directory entries or renames. Recovery verification and backup preservation still apply. This synchronization policy does not enable Windows atomic recovery replacement: the existing no-clobber move policy supports Darwin/Linux and rejects unsupported platforms before mutation.

## Migration snapshots and recovery backups

Before an automatic schema migration, Backscroll publishes a validated sibling
snapshot next to the active database:

```text
<database>.snapshot-v<from>-<signature12>-to-v<to>
```

`signature12` is the first 12 hex characters of the source schema signature, not
a file-content hash. If that name already exists, `.1`, `.2`, etc. distinguish
later publications without overwriting it. After successful publication, pruning
keeps the two newest validated snapshots for that active database (across
migration versions); unrelated/unvalidated lookalikes are not ordinary retained
snapshots. Preserve an additional copy elsewhere if longer retention is needed.
New snapshot files and ownership markers use `0600`, temporary snapshot
directories `0700` on systems supporting POSIX modes; Windows does not promise
Unix permission enforcement. Keep the parent directory trusted.

Recovery backups have a **different** naming and retention contract:
`.<database-basename>.backup-<UTC>-<randomhex>` beside the active database (no
extra leading dot if its basename already has one). They preserve the previous
active file and applicable SQLite sidecars during recovery replacement. Recovery
does not apply the migration two-snapshot pruning policy to these backups;
retain them until replacement validation and operator review are complete.
The report's actual `backup path` is authoritative, not a filename you construct.

A snapshot can be inspected as a stranded recovery input with
`backscroll recover --from "$SNAPSHOT" --dry-run`, then deliberately merged if compatible.
That is lossless union recovery, **not rollback to the snapshot**. For exact
restoration of an earlier active state, follow the manual restoration procedure
below. Never overwrite an active database while a Backscroll or SQLite process
is using it, or mix database files with sidecars from a different state.

## Recovery runbook

Recovery is remediation, not a missing-input discovery command:

1. Stop processes that might write the stranded database; do not bypass a busy
   startup lease. Identify the effective active path from configuration
   (`BACKSCROLL_DATABASE_PATH` or app config). `backscroll config --json` is useful when
   ordinary startup can open that index, but may itself fail on an incompatible
   active index. Both the active database and one `--from` input must already
   exist. Recovery does not create an empty active input for you.
2. Preserve the active and stranded SQLite file sets, including any sidecars,
   in a trusted directory. Then inspect the immutable-input plan:

   ```bash
   backscroll recover --from "$STRANDED_DB" --dry-run
   ```

   Dry-run reports shapes, row counts, duplicates/conflicts, replacement target,
   and an intended backup naming pattern. It does not migrate either input,
   create a replacement/backup, or sync configured sources. CLI coordination
   still uses the startup lock sidecar; dry-run is not a guarantee of zero
   filesystem activity. It is a point-in-time plan, not an apply reservation.
3. Review diagnostics. Only if the lossless union is intended, apply:

   ```bash
   backscroll recover --from "$STRANDED_DB"
   backscroll validate --json
   backscroll status --json
   ```

   Apply re-plans under its retained lease, verifies a fresh destination,
   preserves a verified active backup, installs the replacement, then performs
   post-install sync. Save the actual active/backup paths and output. The
   stranded input is not consumed. Compatibility conflicts fail rather than
   silently choosing a conflicting row.
4. Interpret failure by phase, not exit code alone. If stderr says
   `recovery replacement installed at: ...` followed by `post-recovery sync`,
   installation succeeded but subsequent sync failed. The replacement remains
   active and the reported backup remains available; **no automatic rollback
   occurs at this stage**. Fix the reported input/decoder/permission problem,
   then run `backscroll status` to retry ordinary sync and `backscroll validate --json` to check the
   prepared index. Do not blindly apply recovery again. Earlier replacement
   failures report their phase/state; some restore the original and others
   leave a restorable backup. Use the emitted `manual recovery backup path`
   only when the diagnostic identifies it as restorable.
5. For intentional exact manual restoration, stop all Backscroll/SQLite users,
   preserve the installed active file set separately, and verify the reported
   backup and its matching sidecars. Restore a **copy** of that backup file set
   to the configured active path in a trusted directory, without leaving the
   replacement's `-wal`, `-shm`, or journal beside it. Keep the original backup
   untouched and restrict file access (POSIX `0600` where applicable). Validate
   after restoration; ordinary startup may migrate and sync it again. If the
   goal is to inspect the exact old bytes without that mutation, use a separate
   copy as a dry-run recovery input first. Retire backups only after explicitly
   accepting the recovered/restored state.


## Normal workflow

```bash
# Show the active manifests and resolved paths.
backscroll config

# Snapshot-read, metadata-read, and mutation owners perform startup sync before the handler.
backscroll search --text "migration plan"
backscroll context --uuid "$UUID" --before 5 --after 5
backscroll list --order timestamp:desc --limit 20
backscroll patterns --kind templates --min-support 5
backscroll status --json
backscroll validate --json
```

Human startup sync writes progress and warnings to stderr. JSON/robot startup progress is discarded so stdout remains machine-readable, and invalid active manifests fail during preflight instead of being silently ignored. Busy followers emit `sync_in_progress` warnings to stderr; read-safe followers use the last committed WAL snapshot, config followers print validated configuration without opening the database, and mutation/remediation followers wait up to five seconds before returning a retryable failure. `backscroll recover --dry-run` reports without post-install sync; `backscroll recover` apply runs post-install sync under the same retained remediation lease before printing its report.

Search may filter discovery to a known input path, but it remains ranked and requires query text. Once a result supplies exact identity, use context for the anchor's SQLite-backed neighborhood. Prefer its non-null UUID; only when the UUID is null, use its exact stored source path and ordinal:

```bash
backscroll search --text "artifact literal" --source-path "*session-id*" --all-projects --json
backscroll context --uuid "$UUID" --before 5 --after 5 --json
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --json
```

## Rebuild semantics

```bash
backscroll rebuild
```

`rebuild` is non-destructive. Mutation-class startup sync runs first and prepares the database. The rebuild handler does not perform a second sync: it re-derives both FTS5 indexes from the perennial `search_items` table, backfills derived templates/corrections/tool events from stored text where possible, and re-resolves project identities. It does not discard sessions whose files have expired.

Use `rebuild` after index-recovery work or when derived search structures need regeneration. It is not a substitute for a removed manual sync command.

## Declarative inputs

Canonical ingestion routes are user-scoped manifests:

```text
<config_dir>/backscroll/inputs/*.inputs.toml
```

`<config_dir>` is non-empty `BACKSCROLL_CONFIG_DIR`, otherwise `$HOME/.config` on every OS (Windows: `<user-home>\.config`), not `XDG_CONFIG_HOME`, macOS Application Support, or `%APPDATA%`. Backscroll does not load project-local input manifests. Application configuration in `backscroll.toml` is separate and does not define canonical ingestion routes.

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

Plans and external Markdown documents are also declared as inputs. Use `decode.format = "markdown_document"` for a whole document or `decode.format = "markdown_sections"` to split on `##` headings. See the [input manifest contract](input-contract.md) for supported fields and decoders.

## Incremental and perennial behavior

Backscroll stores a content hash or reader watermark plus a versioned ingestion-semantics fingerprint for each indexed input. Unchanged files are skipped on later startup syncs only while the decoder epoch, format, source, and reasoning option also match. Available semantic mismatches share the 200-path bounded replay queue; unavailable sources remain untouched. OpenCode is the exception to the file-metadata shortcut: its SQLite watermark is queried on every owner sync because WAL commits may not change `opencode.db` itself. Files with stable message UUIDs are updated append-only; legacy or UUID-less inputs retain wipe-and-reload behavior while the source exists.

Pi reader epoch 2 captures native tool events and result signals when an
unchanged source is replayed. Native session/entry/call identities use canonical
UUID reconciliation and allow a later result to enrich its original call.
Legacy Pi sources without a native session ID keep snapshot semantics.
Unavailable sources keep their existing rows and fingerprint; their missing
native metadata remains unknown.

Codex reader epoch 2 replays native function/custom call metadata through the
same shared 200-path bounded queue. Its canonical search records keep NULL
message UUIDs and snapshot semantics, so appending a paired result refreshes
the call's proven outcome together with ordinary appended prose. Native `call_id`
is stored only as nullable V19 satellite metadata in `tool_events.native_call_id`;
the canonical `message_uuid` association and extraction-policy version are
unchanged. Unavailable history keeps its fingerprint and existing metadata.
Recovery lossily rederives tool events from stored text with unknown native
call identity; ingestion replay restores native metadata from available sources.

Startup coordination uses owner/follower branches: snapshot-read, metadata-read, and mutation owners acquire the canonical lock and perform prepare/migrate/sync before the handler runs; a remediation owner acquires and retains the same mutation-grade lock but bypasses ordinary compatible-open and pre-handler sync; a read-safe follower validates the existing database read-only and continues on a compatible snapshot; metadata-read followers avoid opening the database; mutation and remediation followers wait up to five seconds for ownership or fail retryably with `sync_in_progress`. WAL snapshot followers remain compatible only on the same local host.

The SQLite database is the perennial event store, not a disposable cache. When a source file expires, its indexed rows remain available; the perennial store has no deletion path.

## Machine output

`--json` writes one JSON payload to stdout. JSON and robot startup progress is discarded so stdout stays parseable; human progress and warnings use stderr. Structured diagnostics remain parseable in machine modes. `--robot` output shape is command-specific: `backscroll list` and `backscroll patterns` include command-defined sections, while only `backscroll search "<query>" --robot` guarantees deterministic `result_N_field=value` lines. Search robot string values escape backslash as `\\`, carriage return as `\r`, and newline as `\n` so each field remains one line.

## Noise filtering

Each dedicated reader owns provider-specific record inclusion, text cleanup, and tool extraction. Discovery exclusions in the manifest remove configured paths such as Claude subagent sessions; readers remove provider noise such as system reminders, task notifications, and local command metadata. Pi, OMP, and Codex reasoning is indexed only when its manifest sets `index_reasoning = true`.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Command and any required incremental sync completed |
| `1` | Invalid manifest, incompatible index, permission, parse, or database failure |

## Native OMP sessions

Install `inputs/omp.inputs.toml` into the runtime input directory. The dedicated
`omp` decoder reads only native v3 filesystem JSONL and stores `source = "session"`.
Its canonical `*/*.jsonl` include selects direct project-bucket files, not nested
child sessions. Missing roots are safe. Custom agent directories, profiles,
XDG layouts, and `--session-dir` need explicit roots in user manifests.

OMP uses header cwd for project identity and namespaces every emitted physical
block by header session ID and entry ID. Stored branch history is included in
physical order, not reduced to the active branch. Unchanged files skip by hash;
appended UUID-bearing rows preserve previous IDs. Removing a source does not
remove perennial recall, and `rebuild` reconstructs FTS from SQLite. Raw call
IDs pair even out-of-order results for native error/exit/interruption and search
echo provenance before serialization. Unsupported/admin/private payloads are
never serialized as fallback text. See the [OMP input contract](input-contract.md#complete-omp-example).
