---
estado: Specified
tipo: task
---
# T001: Fix OpenCodeReader para schema anomalyco/opencode

**Outcome**: [O14 OpenCode Reader](README.md)
**Contribuye a**: el reader parsea correctamente la DB real instalada en el sistema

## Preserva

- INV1: Coverage ≥ 85 %
  - Verificar: `just coverage-summary`
- INV2: Sin cambios de comportamiento en el reader JSONL ni en otros readers
  - Verificar: `go test ./internal/readers/... ./internal/sync/...`

## Contexto

El schema real de anomalyco/opencode (fuente: `~/.opensrc/repos/github.com/anomalyco/opencode/main/packages/opencode/src/session/message-v2.ts` + migración `20260127222353_familiar_lady_ursula/migration.sql`):

- Tabla `message`: `id`, `session_id`, `time_created`, `time_updated`, `data TEXT`
  - `data` = `Omit<MessageV2.Info, "id"|"sessionID">` → contiene `"role"` (literal `"user"` o `"assistant"`)
- Tabla `part`: `id`, `message_id`, `session_id`, `time_created`, `time_updated`, `data TEXT`
  - `data` = `Omit<MessageV2.Part, "id"|"sessionID"|"messageID">` → contiene `"type"` y `"text"` para TextPart; `"ignored"` es bool opcional

Tipos de part relevantes (solo se indexa texto):
- `type:"text"` → `{"type":"text","text":"...","ignored":optional bool,...}` — incluir si `ignored` es nil o false
- todo lo demás (reasoning, tool, step-start, step-finish, compaction, patch, snapshot, …) → ignorar

El reader actual consulta tabla inexistente `messages` con columnas `role`, `parts`, `created_at`/`updated_at` — falla en runtime si la DB tiene datos.

DB del sistema: `~/.local/share/opencode/opencode.db` (actualmente vacía; tests con fixture).

## Alcance

**In**:
1. En `Hash()`: cambiar query a `SELECT MAX(time_updated) FROM message`
2. En `Parse()`: cambiar SQL a JOIN `message` con `part` ordenado por `(m.time_created, p.id)`; escanear `(id, session_id, msg_data, time_created, part_data)`
3. Definir tipos `msgInfoData{Role string}` y `partInfoData{Type, Text string; Ignored *bool}`
4. Loop de scan: acumular text-parts por mensaje usando slices; emitir al cambiar `msgID`
5. Eliminar helpers obsoletos: `openCodePart`, `textData`, `extractTextFromParts`

**Out**:
- No modificar `JsonlReader`, `input_config`, `storage`, ni ningún otro paquete
- No añadir soporte a partes de tipo reasoning/tool como contenido indexable

## Estado inicial esperado

- `internal/readers/opencode_reader.go` existe con implementación rota (queries a `messages`)
- `go test ./internal/readers/...` puede pasar o fallar dependiendo de si el test fixture usa el schema viejo

## Criterios de Aceptación

- AC1: `Hash()` retorna string no-vacío para DB con al menos un mensaje; retorna `"empty"` para DB vacía
- AC2: `Parse()` retorna exactamente los mensajes con `type:"text"` y `ignored != true`; step-start, tool, reasoning se descartan
- AC3: `Parse()` concatena múltiples text-parts del mismo mensaje con `"\n"`
- AC4: `go test ./internal/readers/... -race` sin errores
- AC5: `just check` (gofmt + go vet) pasa

## Fuente de verdad

- `internal/readers/opencode_reader.go`
- Schema confirmado: `~/.opensrc/repos/github.com/anomalyco/opencode/main/packages/opencode/src/session/message-v2.ts`
- Migración inicial: `~/.opensrc/repos/github.com/anomalyco/opencode/main/packages/opencode/migration/20260127222353_familiar_lady_ursula/migration.sql`
