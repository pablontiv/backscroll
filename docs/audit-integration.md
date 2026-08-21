# Downstream Audit Integration Contract

Backscroll owns the perennial corpus and supported CLI query surfaces. A downstream audit tool owns deterministic findings, thresholds, redaction, report rendering, and any ADR or backlog creation.

## Supported operational boundary

Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, list, patterns, status, and validate.
Use `--source-path` on search as a filter, paired with query text, for database-backed retrieval scoped to a known input path.

Use diagnostics at the start of an audit run:

```bash
backscroll status --json
backscroll validate --json
```

Then query through the database-backed CLI surfaces:

```bash
backscroll list --json --all-projects --order timestamp:asc --limit 100
backscroll search --text "permission denied" --json --all-projects
backscroll patterns --kind failures --json --all-projects
```

Human startup progress and warnings use stderr. JSON/robot startup progress is discarded so stdout remains machine-readable, and structured diagnostics stay parseable in machine modes.

## Status JSON

`backscroll status --json` emits one JSON document with these top-level objects:

- `database`: configured path, existence, and size;
- `index`: usability, indexed file/message counts, timestamp, and derived-data counts;
- `config`: configured session directories and active input identifiers.

Status is preflight metadata. It does not expose transcript content.

## Session discovery

`backscroll list --json` returns a JSON object containing `count` and `sessions`. Each session summary includes its path, project, timestamp, and tags. Supported filters are `--project`, `--all-projects`, `--order`, `--limit`, `--offset`, and legacy `--recent`.

`list` does not expose message-level filters such as `--source-path`, `--source`, `--role`, `--after`, `--before`, or `--content-type`.

## Message and tool investigation

The search command returns ranked matching rows. It supports path, source, project, role, date, tag, and content-type filters. For example:

```bash
backscroll search --text "go test" --content-type tool --source-path "*/example/*.jsonl" --json
backscroll search --text "$QUERY" --source-path "*session-id*" --all-projects --json
```

Search is an investigation surface, not an exhaustive corpus export: ranking, limits, and token budgets may omit rows. The current public CLI does not provide an empty-query stream of every stored message. Consumers requiring a complete message-level export must not infer one from `list` or `search`; they need a separately designed read-only API or an explicitly versioned database integration.

## Privacy and raw-content boundary

Backscroll stores normalized message text and serialized tool content in SQLite. The public CLI does not make raw provider JSONL a downstream schema contract. Database-backed retrieval through search with the `--source-path` filter and query text is the supported drill-down path for a known input path. Raw provider files remain ingestion inputs, not the normal audit read boundary.
