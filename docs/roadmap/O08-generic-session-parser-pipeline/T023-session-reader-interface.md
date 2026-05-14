---
id: T023
tipo: task
estado: Pending
titulo: SessionReader interface + registry
outcome: O08
dependencias: [T014]
---

# T023 — `SessionReader` interface + registry

Definir la interfaz que abstrae cualquier parser de sesiones y el registry que
permite registrar y seleccionar parsers por nombre/formato.

## Alcance

En `internal/readers/reader.go`:

```go
// SessionReader abstrae la lectura de sesiones desde cualquier fuente.
type SessionReader interface {
    // Name retorna el identificador del reader (e.g., "jsonl", "opencode").
    Name() string
    // Discover retorna los paths/IDs de sesiones disponibles según el InputDefinition.
    Discover(def input_config.InputDefinition) ([]string, error)
    // Hash retorna el hash del estado actual de una sesión (para dedup incremental).
    Hash(sessionRef string) (string, error)
    // Parse parsea una sesión y retorna sus mensajes como ParsedFile.
    Parse(sessionRef string, def input_config.InputDefinition) (models.ParsedFile, error)
}

// Registry mapea nombres de readers a sus implementaciones.
type Registry struct { ... }

func NewRegistry() *Registry
func (r *Registry) Register(reader SessionReader)
func (r *Registry) Get(name string) (SessionReader, bool)
func (r *Registry) Default() SessionReader  // retorna "jsonl" reader
```

## Criterios de aceptación

- Interface compilable con los métodos definidos
- Registry con `Register`/`Get` funcional (test unitario)
- Mock reader implementa la interface (usado en tests de O08)
- `go test ./internal/readers/...` pasa
- No se rompe ningún package existente al agregar el nuevo package

## Notas de diseño

- `sessionRef` es un path para JSONL, un ID de sesión para SQLite
- `ParsedFile` ya existe en `internal/models/` — reutilizar sin modificar
