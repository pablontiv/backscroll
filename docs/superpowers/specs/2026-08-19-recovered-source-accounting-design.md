# Recovered Source Accounting Design

**Date:** 2026-08-19  
**Issue:** [#35 — recovered index immediately fails validate as orphaned](https://github.com/pablontiv/backscroll/issues/35)

## Summary

`backscroll recover` currently installs the canonical union of records from the active and stranded databases while leaving `indexed_files` empty. The resulting database passes recovery's private verifier but immediately fails the public `validate --indexed-only` orphan check.

Recovery will add explicit provisional source accounting to `indexed_files`. Each distinct `search_items.source_path` in the recovery plan will receive the reserved marker `backscroll:recovered` instead of a content hash. Consumers will distinguish this marker from a real SHA-256: validation will accept the accounted path, derived-data backfill will continue treating it as recoverable stored data, and autosync will not treat it as an unchanged source. A later real sync will replace the marker with the source file's actual hash.

This change reconstructs database accounting only. It never creates, restores, or modifies session JSONL files.

## Goals

- Make a successfully recovered destination pass `validate --indexed-only` immediately.
- Preserve detection of genuine orphaned `search_items` rows.
- Represent unknown post-recovery source state explicitly without inventing a SHA-256.
- Keep recovered rows eligible for `BackfillDerived()` when their original sources are absent.
- Force a source that does exist on disk to be parsed on the next sync.
- Preserve canonical rows, FTS queryability, atomic installation, and the byte-identical active backup guarantee.

## Non-goals

- Recovering, recreating, or modifying original JSONL or Markdown source files.
- Preserving permanent historical provenance after a source has been synced normally.
- Introducing a new schema table or migration.
- Relaxing public validation for unaccounted rows.
- Recovering lossy derived metadata that is absent from canonical recovery records.

## Existing Invariant Conflict

Three existing behaviors conflict:

1. `verifyRecoveryDestinationRows` requires zero `indexed_files` rows in a recovery destination.
2. `Database.Validate` requires every `search_items.source_path` to exist in `indexed_files`.
3. `BackfillDerived` uses absence from `indexed_files` to identify stored rows whose source is no longer available through normal sync.

The fix must account for recovered paths for validation without making them look synchronized or excluding them from backfill.

## Design

### Reserved accounting marker

Storage will define one internal reserved value:

```text
backscroll:recovered
```

The value is deliberately outside the lowercase 64-character hexadecimal format produced by SHA-256. It means:

> Records for this source path were reconstructed from a database recovery plan; no current content hash for the original source is known.

The marker is provisional. When `SyncFiles` later processes the corresponding source, its existing `INSERT OR REPLACE` into `indexed_files` replaces the marker with the real hash.

### Recovery destination creation

`CreateRecoveryDestination` will continue inserting every canonical recovery record into `search_items` inside the existing transaction. Before destination verification and commit, it will also:

1. Collect each distinct `SourcePath` from the applicable recovery plan.
2. Insert exactly one `indexed_files` row per distinct path.
3. Store `backscroll:recovered` as the row's `hash`.
4. Store `NULL` for `last_indexed`, because recovery did not index the original source at that time.

The table's primary key on `path` provides uniqueness. No source file is read or written during this operation.

### Recovery destination verification

`verifyRecoveryDestinationRows` will replace its current `indexed_files count = 0` rule with exact provisional-accounting verification:

- the destination has one accounting row per distinct planned `SourcePath`;
- every expected path exists;
- every expected row contains `backscroll:recovered`;
- no additional `indexed_files` rows exist.

Any missing path, unexpected path, duplicate-equivalent accounting mismatch, or incorrect marker fails destination verification before installation. Existing canonical-record and FTS verification remains unchanged.

### Public validation

`Database.Validate` retains its current orphan invariant: every `search_items.source_path` must have a corresponding `indexed_files.path`.

Recovered destinations now satisfy that invariant through explicit provisional accounting. A row inserted without either normal or recovery accounting remains a genuine orphan and continues to fail validation.

### Incremental sync

`GetFileHashes` supplies the skip map used by autosync. It will omit rows whose hash is `backscroll:recovered`.

Consequences:

- If an original source still exists, discovery and hashing proceed normally; because the path is absent from the returned skip map, the reader parses it and `SyncFiles` replaces the marker with its real SHA-256.
- If an original source no longer exists, discovery never returns it; the recovered database rows remain perennial and untouched.

No special branch is required in `SyncFiles`: its existing `INSERT OR REPLACE` behavior performs the state transition atomically with the normal sync transaction.

### Status accounting

`GetStats` will exclude `backscroll:recovered` rows from `TotalFiles` and from the `IndexedAt` maximum. Those fields describe sources actually indexed from disk, whereas recovered database records are reported by the existing message counts. This prevents recovery time or provisional paths from masquerading as a successful source sync.

### Derived-data backfill

`BackfillDerived` will classify a source path as recovery/expired input when either:

- no matching `indexed_files` row exists, or
- the matching row's hash is `backscroll:recovered`.

All existing missing-derivation predicates remain in effect. This preserves idempotency and prevents repeated work after templates, correction signals, and lossy tool events have been derived.

### Purge lifecycle

`Purge` already removes an `indexed_files` entry when no `search_items` remain for its path. The same behavior applies to provisional recovery markers, so no marker remains after the last recovered row for a path is purged.

## Data Flow

```text
active DB + stranded DB
          |
          v
 canonical recovery plan
          |
          +--> search_items: canonical recovered rows
          |
          +--> indexed_files: one path -> backscroll:recovered marker
          |
          v
 destination verification -> atomic installation -> validate succeeds

Later autosync:
  source absent  -> recovered rows and marker remain; backfill may derive data
  source present -> GetFileHashes omits marker -> parse -> SyncFiles writes real SHA-256
```

## Error Handling

- Marker insertion failures abort and roll back destination creation.
- Accounting verification failures reject the destination before installation.
- The independent post-commit immutable verification applies the same accounting rules.
- Errors continue using the existing recovery destination error wrappers and cleanup behavior.
- Autosync and backfill database errors keep their existing contextual wrapping.
- No fallback invents or accepts a normal-looking content hash.

## Testing

### CLI regression

Add a full command-level regression using supported active and stranded databases:

1. Capture the active database bytes.
2. Run `recover --from <stranded>`.
3. Run `validate --indexed-only` against the installed destination and require success.
4. Confirm the reported backup is byte-identical to the original active database.
5. Confirm all expected recovered records remain queryable.

### Storage tests

Cover these invariants directly:

- Recovery creates one marked accounting row per distinct planned source path.
- Destination verification rejects a missing accounting row.
- Destination verification rejects an incorrect marker.
- Destination verification rejects an unexpected accounting path.
- Messages and tool records remain queryable through their respective FTS indexes.
- `GetFileHashes` returns real hashes but omits provisional recovery markers.
- `GetStats` excludes provisional markers from file count and last-indexed time.
- `BackfillDerived` processes a marked recovered path.
- `SyncFiles` replaces a marker with the real source hash.
- `Purge` removes a marker after deleting the final row for its path.
- The existing genuine-orphan validation regression continues to fail as expected.

### Verification commands

```bash
just check
just test
just ci
```

## Documentation Impact

This change adds no CLI option and no migration. Internal comments should document the marker contract where it is defined and where queries intentionally include or exclude it. The repository architecture documentation does not need a new package entry because no package is added or removed.
