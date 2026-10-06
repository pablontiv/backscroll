# Downstream Audit Integration Contract

Backscroll owns the perennial corpus and supported CLI query surfaces. A downstream audit tool owns deterministic findings, thresholds, redaction, report rendering, and any ADR or backlog creation.

## Supported operational boundary

Every operational command validates active manifests and attempts one incremental
sync before executing. Session, plan, and Markdown files are ingestion inputs;
SQLite is the perennial record used by search, context, list, patterns, status, and validate.
Use `search` for ranked discovery. When an audit item already has a UUID or exact stored source path and ordinal, use `context` for its exact neighborhood.

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
backscroll context --uuid "$UUID" --before 5 --after 5 --json
```

The first three commands discover or aggregate. The final command is required once exact record identity exists; if UUID is unavailable, use `backscroll context --source-path "$SOURCE_PATH" --ordinal "$ORDINAL" --json`. Do not approximate an exact audit window with another ranked search.

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

Search is an investigation surface, not an exhaustive corpus export: ranking, limits, and token budgets may omit rows. `context` is exact but local: it resolves one opaque UUID or exact source-path-plus-ordinal selector and returns up to 50 neighboring records on either side (five by default). Its complete JSON envelope contains `anchor`, full-field `records`, `truncated`, and `omitted`; each text is capped at 4000 Unicode code points. Its successful-payload budget defaults to 2000 tokens and accepts 64–16384. Diagnostics `context_not_found`, `context_ambiguous`, and `context_budget_too_small` are exempt from that budget.

The current public CLI does not provide an empty-query stream of every stored message. Consumers requiring a complete message-level export must not infer one from `list`, `search`, or `context`; they need a separately designed read-only API or an explicitly versioned database integration.

## Privacy and raw-content boundary

Backscroll stores normalized message text and serialized tool content in SQLite. The public CLI does not make raw provider JSONL a downstream schema contract. `context` reads perennial `search_items` rows only, including compatible rows retained or installed by recovery; it never falls back to raw files. Record origin is parser-backed and may be `unknown` when native evidence is insufficient. Raw provider files remain ingestion inputs, not the normal audit read boundary.
