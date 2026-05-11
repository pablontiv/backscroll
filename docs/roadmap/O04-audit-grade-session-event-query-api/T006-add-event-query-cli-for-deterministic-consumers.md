---
estado: Specified
tipo: task
---
# T006: Add event query CLI for deterministic consumers

**Outcome**: [Audit-grade session event query API](README.md)

[[blocked_by:./T003-add-ordered-indexed-session-detail-query.md]]
[[blocked_by:./T005-preserve-pi-claude-tool-events-during-ingestion.md]]

## Preserva

- Event query is a local structured read surface, not an LLM or semantic discovery mechanism.
- Search/top-k remains available as a supplemental retrieval feature.

## Contexto

Pinata session-audit should query all relevant normalized events and run deterministic buckets/thresholds itself. It needs a stable CLI stream from Backscroll rather than raw JSONL or semantic search results.

## Alcance

**In**:
1. Add a command such as `backscroll events query --project ... --jsonl --indexed-only`.
2. Support filters by project, provider/input, source path/session, date/recent, event type, and limit where appropriate.
3. Emit schema-versioned JSONL in stable source/session/ordinal order.

**Out**:
1. Implementing Pinata session-audit rules in Backscroll.
2. Creating ADRs, KEs, or roadmap items automatically.

## Estado inicial esperado

No Backscroll command currently streams normalized session events as a complete deterministic corpus for external audit consumers.

## Criterios de Aceptación

- `backscroll events query ... --jsonl --indexed-only` or chosen equivalent returns complete normalized events for the selected scope.
- JSONL records include schema version, session/source refs, event type, ordering fields, and event-specific metadata.
- Golden tests validate filter combinations, ordering, empty results, and no-sync behavior.

## Fuente de verdad

- src/main.rs
- src/storage/sqlite.rs
- src/core/sync.rs
- tests/cli.rs
