---
id: T027
tipo: task
estado: Pending
titulo: OpenCodeReader — reader SQLite para .opencode/opencode.db
outcome: O08
dependencias: [T023]
---

# T027 — `OpenCodeReader`: reader SQLite para `.opencode/opencode.db`

Implementar el parser de sesiones OpenCode que lee directamente de la base de datos
SQLite que OpenCode mantiene en `<project_dir>/.opencode/opencode.db`.

## Alcance

En `internal/readers/opencode_reader.go`:

```go
type OpenCodeReader struct{}

func (r *OpenCodeReader) Name() string  // "opencode"
func (r *OpenCodeReader) Discover(def input_config.InputDefinition) ([]string, error)
func (r *OpenCodeReader) Hash(dbPath string) (string, error)
func (r *OpenCodeReader) Parse(dbPath string, def input_config.InputDefinition) (models.ParsedFile, error)
```

### Schema de OpenCode (lectura, no escritura)

```sql
-- sessions: id, title, created_at, updated_at, ...
-- messages: id, session_id, role, parts TEXT (JSON array), model, created_at, updated_at
```

El campo `parts` es un JSON array tipado con objetos `{"type": "text"|"tool_call"|..., "data": {...}}`.
Extraer solo partes de tipo `text` para indexación.

### Discover

- Recorre los `discover.include` dirs buscando `**/.opencode/opencode.db`
- Retorna paths absolutos a los archivos `.db` encontrados

### Hash

- `MAX(updated_at)` de la tabla `messages` como watermark de cambio
- Formatear como string hex para comparación

### Parse

- Leer todas las sesiones del DB
- Para cada sesión: leer mensajes, extraer `text` parts del JSON `parts`
- Mapear `role`: `user`/`assistant` (OpenCode ya usa estos valores)
- Retornar como `ParsedFile` con `Source = "session"`, `SessionID` del DB de OpenCode

## Criterios de aceptación

- `backscroll sync` con `opencode.inputs.toml` activo indexa sesiones de OpenCode
- Mensajes con `parts` de tipo `text` aparecen como `TextContent` indexado
- Partes `tool_call`, `tool_result`, `finish` no se indexan como texto
- Dedup: si `MAX(updated_at)` no cambió, el DB se skipea
- `go test ./internal/readers/...` pasa (con fixture DB de test)

## Notas

- Usar `modernc.org/sqlite` (ya en go.mod) — no agregar nueva dependencia
- El DB de OpenCode se abre en modo read-only (`PRAGMA query_only = ON`)
- No necesita preset TOML en esta task — el reader se puede invocar directamente;
  el preset `opencode.inputs.toml` se crea en una task de O11 o como subtarea
