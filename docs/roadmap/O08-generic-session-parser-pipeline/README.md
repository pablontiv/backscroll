---
id: O08
tipo: outcome
estado: Pending
titulo: Generic Session Parser Pipeline en Go
descripcion: Interfaz SessionReader + registry de parsers. Permite agregar nuevos formatos (JSONL, SQLite) sin modificar el core de sync. Habilita Pi declarativo y OpenCode como primer parser SQLite nativo.
---

# O08 — Generic Session Parser Pipeline en Go

Port del pipeline genérico de `src/core/sync.rs` al Go port. Introduce la interfaz
`SessionReader` que abstrae discover/hash/parse para cualquier formato de sesión.
Permite que Claude JSONL, Pi JSONL y OpenCode SQLite sean parsers intercambiables.

**Depende de**: O07 (tipos y predicados del input manifest engine)

## Tasks

- [T023](T023-session-reader-interface.md) — `SessionReader` interface + registry
- [T024](T024-jsonl-reader.md) — `JsonlReader` (refactor del sync actual)
- [T025](T025-jsonpath-field-selectors.md) — JSONPath-based field selectors
- [T026](T026-pi-parser-declarativo.md) — Pi como primer parser declarativo nativo
- [T027](T027-opencode-sqlite-reader.md) — `OpenCodeReader`: reader SQLite para `.opencode/opencode.db`
- [T028](T028-session-events-extraction.md) — Extracción de session events (timestamp por mensaje)
- [T029](T029-refactor-sync-pipeline-generico.md) — Refactor `sync` para usar pipeline genérico
- [T030](T030-tests-integracion-pipeline.md) — Tests de integración del pipeline

## Criterios de cierre

- `backscroll sync` usa `SessionReader` registry internamente
- `backscroll sync` con OpenCode reader indexa sesiones de `.opencode/opencode.db`
- Comportamiento para Claude y Pi idéntico al actual (tests de regresión)
- `go test ./...` pasa con coverage ≥85%
