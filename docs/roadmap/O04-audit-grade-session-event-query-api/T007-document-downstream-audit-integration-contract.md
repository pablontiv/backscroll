---
estado: Specified
tipo: task
---
# T007: Document downstream audit integration contract

**Outcome**: [Audit-grade session event query API](README.md)

[[blocked_by:./T006-add-event-query-cli-for-deterministic-consumers.md]]

## Preserva

- Backscroll remains the corpus/query layer and does not promise Pinata-specific report semantics.
- Privacy defaults avoid leaking full raw transcripts or absolute private paths in examples unless explicitly opted in.

## Contexto

The product boundary must be clear: Backscroll owns corpus normalization and structured reads; Pinata session-audit owns deterministic findings, redaction, and reporting.

## Alcance

**In**:
1. Document indexed-only/no-sync semantics and status JSON.
2. Document session detail and event query output schemas with examples.
3. Explain that search/topics/insights are supplemental and not exhaustive corpus scans.
4. Include a conceptual Pinata session-audit integration example.

**Out**:
1. Implementing the Pinata adapter.
2. Documenting automatic ADR/KE creation as a supported behavior.

## Estado inicial esperado

Backscroll docs cover input config and search/sync behavior, but do not define an audit-grade downstream integration contract.

## Criterios de Aceptación

- Docs show the recommended deterministic audit flow using indexed-only status/session/event queries.
- Docs state that semantic search/top-k is not sufficient for exhaustive audit discovery.
- Docs identify privacy boundaries, bounded snippets, and raw-content opt-in expectations.

## Fuente de verdad

- README.md
- docs/sync.md
- docs/input-contract.md
- docs/configuration.md
