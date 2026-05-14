---
id: T028
tipo: task
estado: Completed
titulo: Extracción de session events (timestamp por mensaje)
outcome: O08
dependencias: [T023]
---

# T028 — Extracción de session events (timestamp por mensaje)

Actualmente los mensajes se indexan con el timestamp de la sesión (del filename).
Esta task agrega extracción de timestamp individual por mensaje para soportar
`events query` (O11.T045) y filtrado temporal preciso.

## Alcance

- Agregar campo `Timestamp int64` a `models.ParsedFile.Messages` (o `MessageRecord`)
  si no existe ya — verificar schema actual
- En `JsonlReader.Parse()`: extraer timestamp del campo definido en `MapConfig.Timestamp`
  (e.g., `$.timestamp` en Claude JSONL)
- En `OpenCodeReader.Parse()`: usar `messages.created_at` como timestamp por mensaje
- Almacenar en `session_events` table (verificar si existe; si no, nueva migración)
- `session_events`: `(session_id, message_idx, timestamp, role, content_hash)`

## Criterios de aceptación

- `backscroll search "query" --after 2024-01-01` filtra por timestamp de mensaje individual
  (no solo por timestamp de sesión)
- `session_events` tiene una fila por mensaje indexado
- Tests verifican que timestamps se extraen correctamente de Claude y OpenCode
- Nueva migración si se necesita tabla; versión incrementada correctamente

## Notas

- Si `session_events` ya existe en el schema (verificar `internal/storage/migrations.go`),
  solo agregar las columnas faltantes via migración nueva
