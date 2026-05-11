---
estado: Completed
tipo: outcome
---
# Audit-grade session event query API

Backscroll exposes a read-only, deterministic, structured query surface for downstream consumers such as Pinata session-audit. Consumers can scan indexed sessions and normalized events without relying on semantic top-k search or private Pi/Claude JSONL parsing.

## Criterios de Aceptación

- Backscroll can answer status and inventory queries from the existing index without triggering sync.
- Backscroll can stream ordered indexed session records and normalized session events in stable versioned JSON/JSONL.
- Pinata session-audit and similar consumers have a documented contract for deterministic corpus scans, evidence snippets, and privacy-safe downstream reporting.

## Tasks

| Task | Descripción |
|------|-------------|
| [T001](T001-add-indexed-only-snapshot-mode.md) | Add an explicit read-only snapshot mode for Backscroll read commands so deterministic consumers can inspect the current index without triggering discovery or sync. |
| [T002](T002-add-structured-status-json.md) | Expose Backscroll status as stable machine-readable JSON so tools do not parse human text output for database, input, project, and count metadata. |
| [T003](T003-add-ordered-indexed-session-detail-query.md) | Add a direct query for ordered indexed session records so consumers can scan session content deterministically without semantic search ranking. |
| [T004](T004-add-normalized-session-event-model-and-storage.md) | Extend Backscroll's indexed representation with audit-oriented normalized events for messages, tool calls, tool results, commands, errors, and metadata. |
| [T005](T005-preserve-pi-claude-tool-events-during-ingestion.md) | Adapt Backscroll ingestion so current Pi and Claude session formats produce normalized tool, command, result, and error events. |
| [T006](T006-add-event-query-cli-for-deterministic-consumers.md) | Add a direct event query command for consumers that need complete deterministic scans of normalized events within a project/session scope. |
| [T007](T007-document-downstream-audit-integration-contract.md) | Document how downstream tools should use Backscroll's indexed-only status, session detail, and normalized event query APIs for deterministic audit workflows. |
