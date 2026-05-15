---
estado: Completed
tipo: task
---
# T003: Actualizar tests de OpenCodeReader al schema nuevo

**Outcome**: [O14 OpenCode Reader](README.md)
**Contribuye a**: el reader tiene cobertura verificada contra el schema real

[[blocked_by:./T001-fix-reader-schema.md]]

## Preserva

- INV1: Coverage ≥ 85 %
  - Verificar: `just coverage-summary`
- INV2: Tests no requieren OpenCode instalado (usan fixture en memoria/temp)
  - Verificar: `go test ./internal/readers/...` en CI sin OpenCode

## Contexto

`internal/readers/opencode_reader_test.go` puede existir con fixtures del schema viejo (`messages` con columna `parts`). Necesita actualizarse para el schema nuevo: tabla `message` con blob `data` y tabla `part` con blob `data`.

Schema mínimo para el fixture:
```sql
CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL,
    time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL);
CREATE TABLE part (id TEXT PRIMARY KEY, message_id TEXT NOT NULL,
    session_id TEXT NOT NULL, time_created INTEGER NOT NULL,
    time_updated INTEGER NOT NULL, data TEXT NOT NULL);
```

Datos de prueba:
- 1 mensaje usuario: `data = '{"role":"user","time":{"created":1000}}'`, `time_created=1000`, `time_updated=1000`
- 2 parts del mensaje: `{"type":"text","text":"hola mundo"}` y `{"type":"step-start"}` (debe filtrarse)
- 1 mensaje asistente con part `{"type":"text","text":"respuesta","ignored":true}` (debe filtrarse)
- Para Hash: 1 mensaje con `time_updated=999` para verificar el hex resultante

## Alcance

**In**:
1. Crear o reescribir `internal/readers/opencode_reader_test.go`
2. Helper `newTestDB(t) string` que crea SQLite en `t.TempDir()` con schema + datos de fixture
3. `TestOpenCodeReaderHash`: DB vacía → `"empty"`; DB con datos → hex no-vacío
4. `TestOpenCodeReaderParse`: verifica filtraje de step-start, filtraje de ignored=true, concatenación de text-parts, normalización de roles

**Out**:
- No añadir tests de integración contra la DB real en `~/.local/share/opencode/`
- No modificar tests de otros packages

## Estado inicial esperado

- Puede existir `internal/readers/opencode_reader_test.go` con fixtures del schema viejo; si existe hay que reescribirlo completamente
- `modernc.org/sqlite` ya es dependencia del módulo (importada en `internal/storage/`)

## Criterios de Aceptación

- AC1: `go test ./internal/readers/... -v -run TestOpenCodeReader` muestra PASS para Hash y Parse
- AC2: Parse devuelve exactamente 1 mensaje (el user con "hola mundo"); el mensaje asistente (ignored) y los parts no-text se descartan
- AC3: `go test ./internal/readers/... -race` sin data races
- AC4: `just coverage-summary` reporta ≥ 85 % global

## Fuente de verdad

- `internal/readers/opencode_reader.go` (después de T001)
- `internal/storage/unit_test.go` y `internal/readers/` (patrones de test existentes)
- `go.mod` (confirmar que `modernc.org/sqlite` está disponible)
