---
estado: Completed
tipo: task
---
# T003: Add ordered indexed session detail query

**Outcome**: [Audit-grade session event query API](README.md)

[[blocked_by:./T001-add-indexed-only-snapshot-mode.md]]

## Preserva

- Semantic `search`, `topics`, and `insights` remain optimized for retrieval UX, not exhaustive audit scans.
- Output is bounded/redacted enough for local tooling defaults and does not create persistent export artifacts.

## Contexto

Deterministic audit discovery needs complete ordered records within a project/session scope. Top-k search cannot guarantee recall, and `list` only exposes session metadata.

## Alcance

**In**:
1. Add a command such as `backscroll sessions show/query ... --jsonl` for indexed session records.
2. Support filters by project, source path/session, source/input, recency/date, and limit where appropriate.
3. Emit deterministic ordering by session/source path and ordinal/timestamp.

**Out**:
1. Normalizing tool_call/tool_result events beyond what is already indexed as message records.
2. Implementing Pinata session-audit integration.

## Estado inicial esperado

Backscroll can list sessions and search matching chunks, but it cannot stream all indexed records for a scope in stable order.

## Criterios de Aceptación

- A CLI command streams indexed session records as JSONL without requiring a search query.
- Each record includes schema version, project/source identifiers, source path or stable ref, role/content type, timestamp if known, ordinal/order, and bounded text/snippet.
- Golden tests prove deterministic order and filter behavior.

## Fuente de verdad

- src/storage/sqlite.rs
- src/core/sync.rs
- src/main.rs
- tests/cli.rs
