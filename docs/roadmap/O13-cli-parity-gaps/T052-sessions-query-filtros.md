---
id: T052
tipo: task
estado: Pending
titulo: sessions query — filtros faltantes
outcome: O13
dependencias: []
---

# T052 — `sessions query` filtros faltantes

Añadir los flags de filtrado que tiene v0 Rust pero están ausentes en el Go port para `backscroll sessions query`.

## Alcance

En `internal/storage/records.go` — extender `IndexedRecordQuery`:
- `SourcePath *string` ya existe — verificar que soporta LIKE pattern (`*` → `%`)
- Añadir `MaxChars int` — si >0, truncar `Text` a esa cantidad de caracteres en `QueryIndexedRecords`

En `cmd/backscroll/sessions.go` — añadir flags a `newSessionsQueryCmd`:
- `--source-path string` — filtrar por path (LIKE soportado)
- `--max-chars int` (default: 2000) — máximo de caracteres por registro de texto
- `--indexed-only bool` — abrir DB en modo read-only sin auto-sync

Verificar que SourcePath en `QueryIndexedRecords` convierte `*` → `%` y usa LIKE cuando hay wildcard.

## Criterios de aceptación

- `backscroll sessions query --source-path "*.jsonl"` filtra por path con glob
- `backscroll sessions query --max-chars 500` trunca text a 500 chars
- `backscroll sessions query --indexed-only` no hace auto-sync
- Tests de integración para cada nuevo flag
- Coverage ≥85% mantenido
