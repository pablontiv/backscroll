# Downstream Audit Integration Contract

Backscroll owns the perennial corpus and supported CLI query surfaces. A downstream audit tool owns deterministic findings, thresholds, redaction, report rendering, and any ADR or backlog creation.

## Establish a read-only boundary

Use diagnostics before reading an existing snapshot:

```bash
backscroll status --json
backscroll validate --json
```

Both commands are always read-only and never run input discovery or sync. The deprecated `--indexed-only` flag is accepted on them but does not change behavior.

For query commands, `--indexed-only` suppresses auto-sync and reads the configured SQLite snapshot:

```bash
backscroll list --json --indexed-only --all-projects --order timestamp:asc --limit 100
backscroll search --text "permission denied" --json --indexed-only --all-projects
backscroll patterns --kind failures --json --indexed-only --all-projects
```

Without `--indexed-only`, `list`, `search`, and `patterns` validate active manifests and incrementally index changed inputs before querying.

## Status JSON

`backscroll status --json` emits one JSON document with these top-level objects:

- `database`: configured path, existence, and size;
- `index`: usability, indexed file/message counts, timestamp, and derived-data counts;
- `config`: configured session directories and active input identifiers.

Status is preflight metadata. It does not expose transcript content.

## Session discovery

`backscroll list --json --indexed-only` returns a JSON object containing `count` and `sessions`. Each session summary includes its path, project, timestamp, and tags. Supported filters are `--project`, `--all-projects`, `--order`, `--limit`, `--offset`, and legacy `--recent`.

`list` does not expose message-level filters such as `--source-path`, `--source`, `--role`, `--after`, `--before`, or `--content-type`.

## Message and tool investigation

`backscroll search` requires a non-empty query and returns ranked matching rows. It supports path, source, project, role, date, tag, and content-type filters. For example:

```bash
backscroll search --text "go test" --content-type tool --source-path "*/example/*.jsonl" --indexed-only --json
```

Search is an investigation surface, not an exhaustive corpus export: ranking, limits, and token budgets may omit rows. The current public CLI does not provide an empty-query stream of every stored message. Consumers requiring a complete message-level export must not infer one from `list` or `search`; they need a separately designed read-only API or an explicitly versioned database integration.

## Privacy and raw-content boundary

Backscroll stores normalized message text and serialized tool content in SQLite. The public CLI does not make raw provider JSONL a downstream schema contract. `backscroll read --path <FILE>` directly parses a user-supplied file when raw-source access is intentional, but it requires that file to remain on disk and is separate from indexed snapshot reads.
