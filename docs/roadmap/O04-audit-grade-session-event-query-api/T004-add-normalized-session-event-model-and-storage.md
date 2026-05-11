---
estado: Completed
tipo: task
---
# T004: Add normalized session event model and storage

**Outcome**: [Audit-grade session event query API](README.md)

## Preserva

- Search UX is not polluted with full tool outputs by default.
- Raw provider JSONL remains an ingestion input detail, not the downstream API contract.

## Contexto

Session-audit needs ordered commands, tool calls, tool results, exit/error evidence, and decision text. Those structures should be normalized by Backscroll instead of each consumer re-parsing provider internals.

## Alcance

**In**:
1. Design a versioned normalized event schema and storage/migration path.
2. Represent at least message, tool_call, tool_result, command, error, and metadata/other events.
3. Include provider/input id, project, session/source ref, ordinal, timestamp, actor/role, tool name/id, command, cwd, exit code/is_error, and bounded snippet where applicable.

**Out**:
1. Indexing unlimited raw tool output into search by default.
2. Implementing downstream Pinata candidate heuristics.

## Estado inicial esperado

Backscroll indexes searchable text records, but does not expose a complete normalized session event model suitable for audit consumers.

## Criterios de Aceptación

- A schema-versioned event model is implemented and documented in code/tests.
- Storage migrations or compatibility handling preserve existing indexes or provide clear rebuild guidance.
- Fixtures demonstrate message, tool_call, tool_result, command, and error events in stable order.

## Fuente de verdad

- src/storage/sqlite.rs
- src/core/sync.rs
- src/input_config.rs
- docs/input-contract.md
- tests/fixtures
