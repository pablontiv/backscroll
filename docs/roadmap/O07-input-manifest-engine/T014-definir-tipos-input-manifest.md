---
id: T014
tipo: task
estado: Completed
titulo: Definir tipos Go para *.inputs.toml
outcome: O07
---

# T014 — Definir tipos Go para `*.inputs.toml`

Crear el package `internal/input_config/` con los structs Go que mapean el formato
TOML del manifest de inputs. Referencia canónica: `src/input_config.rs` en branch v0.

## Alcance

Definir en `internal/input_config/types.go`:

```go
type InputManifest struct {
    Name     string
    Enabled  bool
    Inputs   []InputDefinition
}

type InputDefinition struct {
    Discover DiscoverConfig
    Decode   DecodeConfig
    Record   RecordConfig
    Map      MapConfig
    Content  ContentConfig
    Text     TextConfig
}

type DiscoverConfig struct {
    Include        []string
    Exclude        []string
    FollowSymlinks bool
}

type DecodeConfig struct {
    Format string // "jsonl", "json", "markdown"
}

type RecordConfig struct {
    Selector   string // JSONPath
    Predicates []Predicate
}

type MapConfig struct {
    Role      string // JSONPath
    UUID      string // JSONPath
    Timestamp string // JSONPath
    SessionID string // JSONPath
}

type ContentConfig struct {
    Blocks []ContentBlock
}

type ContentBlock struct {
    Role     string
    Selector string // JSONPath
}

type TextConfig struct {
    Transforms []Transform
}

type Predicate struct {
    Field    string
    Operator string // "eq", "ne", "in", "exists", "missing"
    Value    interface{}
}

type Transform struct {
    Type    string // "remove", "trim", "join", "drop_empty"
    Pattern string // regex o substring (para remove)
    With    string // (para join)
}
```

## Criterios de aceptación

- `go build ./internal/input_config/...` sin errores
- Tipos anotados con tags TOML correctos para `go-toml/v2`
- Test básico: unmarshal de `inputs/claude.inputs.toml` en `InputManifest` sin error
- No se rompe ningún test existente (`go test ./...` pasa)

## Referencias

- `src/input_config.rs` en branch v0 (structs `InputDefinition`, `DiscoverConfig`, etc.)
- `inputs/claude.inputs.toml` — preset existente como fixture de prueba
