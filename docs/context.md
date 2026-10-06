# Exact Context Retrieval

`backscroll context` returns the indexed records immediately around one exact anchor. Use it after discovery has produced a stable identity. Use the `search` command when you still need to discover relevant records by words, paths, project, source, role, date, tag, or content type.

## Select an anchor

Supply exactly one selector. A UUID is an opaque identifier: pass the complete value without parsing, shortening, or globbing it.

```bash
backscroll context --uuid "$UUID"
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL"
```

The second form requires the exact stored `source_path` and ordinal. It does not accept path fragments or globs. Selectors must resolve to exactly one indexed row; zero matches produce `context_not_found` and multiple matches produce `context_ambiguous`.

The default window is five records before and five after the anchor. Each side can be set independently from 0 through 50:

```bash
backscroll context --uuid "$UUID" --before 2 --after 8
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 0 --after 0 --json
```

Rows come from the anchor's stored source path and are ordered by ordinal, then database row ID. The anchor is always marked and retained. Every record text is capped at 4000 Unicode code points.

## Output and budget

Text is the default. `--json` and `--robot` are mutually exclusive machine modes.

```bash
backscroll context --uuid "$UUID" --json --max-tokens 2000
backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --robot --max-tokens 2000
```

`--max-tokens` defaults to 2000 and accepts 64 through 16384. It applies to the complete escaped successful payload. If the full window does not fit, whole edge records are removed deterministically while preserving the anchor. `truncated` and `omitted` report that reduction. If the anchor and required metadata cannot fit, no partial success is emitted; the command produces `context_budget_too_small` instead. Structured diagnostics are intentionally exempt from this budget.

JSON success is one envelope:

```json
{
  "anchor": {"uuid": "opaque-uuid", "source_path": "/exact/session.jsonl", "ordinal": 42},
  "records": [
    {
      "uuid": "opaque-uuid",
      "source_path": "/exact/session.jsonl",
      "ordinal": 42,
      "role": "assistant",
      "origin": "assistant",
      "timestamp": "2026-01-02T03:04:05Z",
      "content_type": "text",
      "source": "session",
      "text": "indexed text",
      "is_anchor": true
    }
  ],
  "truncated": false,
  "omitted": 0
}
```

Nullable `uuid` and `timestamp` values are JSON `null`. The `anchor` object always contains `uuid`, `source_path`, and `ordinal`; every record contains all fields shown above. `origin` is parser-backed provenance and is one of `human`, `assistant`, `system`, `automation`, or `unknown`. Backscroll does not infer it from text or a historical role; insufficient evidence remains `unknown`.

Robot success is line-oriented `key=value` data. Envelope keys are `anchor_uuid`, `anchor_source_path`, `anchor_ordinal`, `records`, `truncated`, and `omitted`. For each zero-based record `N`, it emits `record_N_uuid`, `record_N_source_path`, `record_N_ordinal`, `record_N_role`, `record_N_origin`, `record_N_timestamp`, `record_N_content_type`, `record_N_source`, `record_N_text`, and `record_N_is_anchor`. String values escape control characters, quotes, and backslashes. Null UUIDs and timestamps are the literal `null`.

Text success contains the same anchor, truncation metadata, and complete record fields in a readable layout.

## Diagnostics

| Code | Meaning |
| --- | --- |
| `context_not_found` | The exact selector matched no indexed row. |
| `context_ambiguous` | The exact selector matched more than one indexed row. |
| `context_budget_too_small` | A valid budget cannot contain the anchor and required metadata. |

Diagnostics are failures. Text diagnostics go to stderr; JSON and robot diagnostics are structured on stdout so machine output remains parseable.

## Storage boundary

Context reads only perennial `search_items` rows in SQLite. It does not read source files or require an `indexed_files` row. Indexed rows remain available after configured source files expire, and compatible rows preserved or installed by recovery are eligible immediately. There is no raw-file fallback.

Every operational invocation still follows the snapshot-read startup policy: validate active manifests, prepare or migrate the index, attempt incremental sync as the lock owner, then query the committed database snapshot. A busy read-safe follower queries the last committed read-only snapshot.

## Workflow rule

Use discovery only until exact identity exists:

```bash
backscroll search "permission denied" --all-projects --robot --fields full --max-tokens 2000
backscroll context --uuid "$UUID" --before 5 --after 5 --robot --max-tokens 2000
```

Corrections, audit, and read/recovery workflows must switch to `context` when they already have a UUID or exact source-path-plus-ordinal. Do not continue using ranked search to approximate an exact neighborhood, and do not fall back to raw provider files.
