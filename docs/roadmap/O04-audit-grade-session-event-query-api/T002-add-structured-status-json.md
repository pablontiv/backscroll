---
estado: Specified
tipo: task
---
# T002: Add structured status JSON

**Outcome**: [Audit-grade session event query API](README.md)

[[blocked_by:./T001-add-indexed-only-snapshot-mode.md]]

## Preserva

- Existing human-readable `backscroll status` output remains available.
- Status JSON reports metadata; it does not expose private transcript content.

## Contexto

Pinata and Pi integrations currently need preflight/status information, but parsing text output is brittle. A versioned JSON status output provides a stable integration point.

## Alcance

**In**:
1. Add `backscroll status --json` or equivalent structured output.
2. Include schema/version, database path, indexed counts, project counts, active inputs, and last sync/index metadata when available.
3. Cover JSON shape and indexed-only/no-sync behavior with tests.

**Out**:
1. Replacing the existing status table output.
2. Adding event-level export/query data.

## Estado inicial esperado

`backscroll status` prints human text with useful counts, but downstream code cannot consume it reliably as a contract.

## Criterios de Aceptación

- `backscroll status --json` emits valid versioned JSON with stable top-level fields.
- The JSON output includes enough counts and input metadata for preflight checks without scraping text.
- Tests cover both populated and empty-index cases.

## Fuente de verdad

- src/main.rs
- README.md
- tests/cli.rs
