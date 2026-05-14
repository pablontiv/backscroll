---
dependencias: []
estado: Completed
id: T051
outcome: O13
tipo: task
titulo: events query — filtros faltantes
---

# T051 — `events query` filtros faltantes

Añadir los flags de filtrado que tiene v0 Rust pero están ausentes en el Go port para `backscroll events query`.

## Alcance

En `internal/storage/records.go` — extender `SessionEventQuery`:
```go
type SessionEventQuery struct {
    Project    *string
    Source     *string
    SourcePath string
    EventType  *string
    Role       string
    After      string
    Before     string
    Limit      int
}
```
- `Project`: filtrar por `project = ?`
- `Source`: filtrar por `source = ?` (normalizar "all" → nil)
- `SourcePath`: soporte LIKE pattern (`*` → `%`) además de igualdad exacta
- `EventType`: filtrar por `event_type = ?`

En `cmd/backscroll/events.go` — añadir flags a `newEventsQueryCmd`:
- `--project string` — filtrar por proyecto
- `--all-projects bool` — no filtrar por proyecto
- `--source string` (default: "session") — filtrar por fuente
- `--source-path string` — filtrar por path (LIKE soportado)
- `--event-type string` — filtrar por tipo de evento
- `--indexed-only bool` — abrir DB en modo read-only sin auto-sync (usar `storage.OpenReadOnly`)

Lógica de proyecto: si `--all-projects` → Project nil; si `--project` → usar ese; si ninguno → derivar del directorio actual (`os.Getwd()` → replace `/` con `-`).

## Criterios de aceptación

- `backscroll events query <path> --project foo` filtra por proyecto
- `backscroll events query <path> --source session` filtra por fuente
- `backscroll events query <path> --event-type message` filtra por tipo
- `backscroll events query <path> --source-path "*.jsonl"` soporta glob
- `backscroll events query <path> --indexed-only` no hace auto-sync
- Tests de integración para cada nuevo flag
- Coverage ≥85% mantenido
