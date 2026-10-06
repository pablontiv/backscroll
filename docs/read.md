---
estado: Completed
---
# Migration from Direct File Reads to Indexed Retrieval

The former direct-read CLI path has been removed from living guidance. Backscroll has one operational retrieval boundary:

```text
active manifests -> mandatory startup sync -> perennial SQLite -> database-backed query
```

Session, plan, and Markdown files are ingestion inputs. SQLite is the perennial record used by `search`, `context`, `list`, `patterns`, `status`, and `validate`.

## Choose discovery or exact context

Use `search` for discovery when you know terms, a path fragment, project, source, stored role, date, tag, or content type but do not yet have an exact record identity:

```bash
backscroll search --text "$QUERY" --source-path "$SOURCE_PATH" --all-projects --json --fields minimal
backscroll search --text "$QUERY" --content-type tool --all-projects --robot --fields full
```

The search `--source-path` flag filters stored `search_items.source_path`; it accepts exact values or discovery patterns and still requires query text. Search is ranked and must not be treated as an exact neighborhood read. Its `role` field is stored compatibility metadata, not a semantic-origin guarantee.

Every result carries the selectors needed for the next step:

| Search output | Stored path | Nullable UUID | Ordinal |
| --- | --- | --- | --- |
| Text | `Path` | `UUID` | `Ordinal` |
| JSON `--fields minimal` | `source_path` | `uuid` | `ordinal` |
| JSON `--fields full` | `FilePath` | `UUID` | `Ordinal` |
| Robot, minimal or full | `result_N_filepath` | `result_N_uuid` | `result_N_ordinal` |

A missing UUID is `null`; Backscroll does not invent one. If the UUID is not null, use it. If it is null, use the exact path and ordinal from the same result:

```bash
if [ "$UUID" != "null" ] && [ -n "$UUID" ]; then
  backscroll context --uuid "$UUID" --before 5 --after 5 --json
else
  backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --before 5 --after 5 --json
fi
```

The UUID is opaque. The alternate selector requires the exact stored source path and ordinal; no globbing or path fragments are accepted. Supply exactly one selector. If more than one indexed row has that exact coordinate, `context` fails with `context_ambiguous` rather than choosing one.

## Context contract summary

- Window defaults: five records before and five after; each side accepts 0–50.
- Text cap: 4000 Unicode code points per record.
- Budget: `--max-tokens` defaults to 2000 and accepts 64–16384.
- Successful JSON is one `anchor` / `records` / `truncated` / `omitted` envelope; text and robot contain the same record fields.
- `role` is not semantic origin. Separate parser-backed `origin` is `human`, `assistant`, `system`, `automation`, or `unknown`.
- Exact-selector failures are `context_not_found` or `context_ambiguous`; an anchor that cannot fit the successful-payload budget produces `context_budget_too_small`.
- Diagnostics are exempt from the successful-payload token budget.

See [Exact Context Retrieval](context.md) for the complete output schema and machine-mode contract.

## Perennial and recovery behavior

`context` reads only `search_items` in SQLite. It does not parse source files, require an `indexed_files` row, or fall back to raw source. Indexed rows remain queryable after source files expire unless `purge` removes them explicitly. Compatible rows retained or installed by recovery use the same query path.

`list` remains a session/document summary surface. It does not accept message-level selectors or filters. `search` discovers candidate records. `context` returns an exact anchor-relative window.

## Raw-file boundary

Do not fall back to `cat`, `jq`, Python, or filesystem session hunting for normal retrieval. Raw-file techniques are reserved for explicitly authorized indexing-bug diagnosis after database-backed commands and diagnostics have been reported.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Query and required startup handling completed successfully. |
| `1` | Invalid arguments, diagnostic failure, manifest preflight failure, or database/query failure. |
