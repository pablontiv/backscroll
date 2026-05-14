---
id: O12
tipo: outcome
estado: Completed
titulo: OpenCode native parser
descripcion: Parser nativo para sesiones de OpenCode — lee .opencode/opencode.db (SQLite), discovery declarativa, dedup por MAX(updated_at), registrado en el reader registry del sync pipeline.
---

# O12 — OpenCode native parser

Parser nativo para sesiones de OpenCode integrado en el pipeline genérico de O08.

## Implementación

- `internal/readers/opencode_reader.go` — `OpenCodeReader` implementa `SessionReader` (Name, Discover, Hash, Parse)
- Registrado en `cmd/backscroll/sync.go` junto a `JsonlReader`
- Tests en `internal/readers/opencode_reader_test.go`

## Criterios de éxito

- CE1: `go test ./internal/readers/...` pasa ✓
- CE2: `OpenCodeReader` registrado en sync pipeline ✓
- CE3: Discovery, Hash y Parse implementados ✓
