---
id: T024
tipo: task
estado: Completed
titulo: JsonlReader (refactor del sync actual; implementa SessionReader)
outcome: O08
dependencias: [T023, T016]
---

# T024 — `JsonlReader` (refactor del sync actual)

Extraer la lógica de parsing JSONL de `internal/sync/sync.go` en un `JsonlReader`
que implementa la interface `SessionReader`. El comportamiento debe ser idéntico.

## Alcance

En `internal/readers/jsonl_reader.go`:

```go
type JsonlReader struct{}

func (r *JsonlReader) Name() string
func (r *JsonlReader) Discover(def input_config.InputDefinition) ([]string, error)
func (r *JsonlReader) Hash(path string) (string, error)
func (r *JsonlReader) Parse(path string, def input_config.InputDefinition) (models.ParsedFile, error)
```

- `Discover`: delega a `input_config.DiscoverFiles()` con el `DiscoverConfig` del manifest
- `Hash`: SHA-256 del contenido del archivo (equivalente al dedup actual)
- `Parse`: extrae mensajes del JSONL usando `MapConfig` del manifest (fields selectors)
  + aplica predicados del `RecordConfig` + text transforms del `TextConfig`

## Criterios de aceptación

- Tests de regresión: mismas sesiones Claude y Pi que parse el sync actual
- `Hash()` retorna el mismo hash que el dedup actual en `indexed_files`
- Parsear un fixture Claude JSONL produce el mismo `ParsedFile` que `ParseSessions()` actual
- `go test ./internal/readers/...` pasa

## Notas

- Mantener `internal/sync/sync.go` funcionando (no eliminar) hasta T029
- Esta task es un refactor interno — no hay cambio de comportamiento visible
