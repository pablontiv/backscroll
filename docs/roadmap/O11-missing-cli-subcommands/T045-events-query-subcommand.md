---
id: T045
tipo: task
estado: Completed
titulo: events query subcommand
outcome: O11
dependencias: [T028]
---

# T045 — `events query` subcommand

Implementar `backscroll events query` que emite los eventos individuales de una
sesión (mensajes con timestamp, rol y contenido). Equivalente al subcomando del
mismo nombre en v0.

## Alcance

En `cmd/backscroll/events.go`:

```
backscroll events query <session-id|session-path>
  --json       output JSON (default: human-readable)
  --robot      output optimizado para LLM (texto plano, sin decoración)
  --role user|assistant   filtrar por rol
  --after <date>          filtrar por fecha (ISO 8601)
  --before <date>         filtrar por fecha
  --limit N               max eventos (default: 100)
```

Flujo:
1. Resolver `session-id` → path via `indexed_files` o lookup directo
2. Leer mensajes de `session_events` table (T028) o parsear JSONL directamente
3. Emitir uno por línea en JSONL (o human-readable con separadores)

## Criterios de aceptación

- `backscroll events query <id>` emite mensajes individuales de la sesión
- `--json` produce JSONL válido (un objeto por línea)
- `--role user` filtra solo mensajes del usuario
- `--after 2024-01-01` filtra por fecha correctamente
- Error descriptivo si session-id no existe
- `go test ./cmd/backscroll/...` pasa

## Referencias

- `backscroll events query` en v0 CLI (`src/cli/events.rs`)
- `session_events` table (T028)
