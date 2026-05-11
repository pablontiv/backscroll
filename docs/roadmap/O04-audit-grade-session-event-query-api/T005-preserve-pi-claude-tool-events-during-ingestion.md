---
estado: Completed
tipo: task
---
# T005: Preserve Pi/Claude tool events during ingestion

**Outcome**: [Audit-grade session event query API](README.md)

[[blocked_by:./T004-add-normalized-session-event-model-and-storage.md]]

## Preserva

- The generic input engine remains the canonical ingestion path.
- Provider-specific details are isolated to presets/extractors rather than leaked to downstream consumers.

## Contexto

Pinata session-audit currently fails on recent Pi sessions because tool calls/results changed shape. Backscroll should own provider-format normalization once, then expose stable events to consumers.

## Alcance

**In**:
1. Add/update fixtures for current Pi records using `toolCall`, `arguments`, and `message.role == toolResult`.
2. Add/update fixtures for Claude `tool_use` and `tool_result` records.
3. Map bash/tool commands, tool errors, exit codes, and snippets into normalized events.

**Out**:
1. Reintroducing old session parser APIs as the public canonical contract.
2. Building Pinata-specific report generation inside Backscroll.

## Estado inicial esperado

Backscroll can ingest configured session text, but current downstream audit needs provider tool event semantics that are not yet exposed as stable normalized events.

## Criterios de Aceptación

- Current Pi fixtures produce normalized tool_call, tool_result, command, and error events.
- Claude fixtures produce equivalent normalized events for tool use/results.
- Tests cover command extraction, tool result error extraction, exit/error metadata, and normal message preservation.

## Fuente de verdad

- inputs/pi.inputs.toml
- inputs/claude.inputs.toml
- src/input_config.rs
- src/core/sync.rs
- tests/fixtures
