---
id: T015
tipo: task
estado: Pending
titulo: Loader de ~/.config/backscroll/inputs/
outcome: O07
dependencias: [T014]
---

# T015 — Loader de `~/.config/backscroll/inputs/`

Implementar `LoadInputs()` que carga todos los archivos `*.inputs.toml` desde
el directorio canónico de manifests de inputs.

## Alcance

En `internal/input_config/loader.go`:

```go
// LoadInputs carga todos los *.inputs.toml activos desde el directorio de configuración.
// Respeta BACKSCROLL_CONFIG_DIR si está definido; si no, usa os.UserConfigDir().
func LoadInputs() ([]InputManifest, error)

// InputsDir retorna el directorio canónico de inputs.
func InputsDir() (string, error)
```

- Glob `*.inputs.toml` en el directorio resuelto
- Parsear cada archivo con `go-toml/v2`
- Filtrar inputs donde `enabled = false`
- Error descriptivo si el directorio no existe (no fatal — retornar vacío)
- `BACKSCROLL_CONFIG_DIR` override para tests

## Criterios de aceptación

- `LoadInputs()` retorna ≥1 manifest cuando `claude.inputs.toml` está en el dir canónico
- Inputs con `enabled = false` no aparecen en el resultado
- `BACKSCROLL_CONFIG_DIR=/tmp/test backscroll inputs list` usa el dir alternativo
- Error si un archivo TOML tiene sintaxis inválida
- `go test ./internal/input_config/...` pasa

## Referencias

- `InputConfig::load()` en `src/input_config.rs` (v0 branch)
- `internal/config/config.go` — patrón de resolución de paths existente
