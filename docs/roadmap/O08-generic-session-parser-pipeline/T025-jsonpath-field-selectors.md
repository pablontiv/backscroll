---
id: T025
tipo: task
estado: Completed
titulo: JSONPath-based field selectors
outcome: O08
dependencias: [T014]
---

# T025 — JSONPath-based field selectors

Implementar el selector de campos via JSONPath para extraer role, uuid, timestamp,
session_id y content blocks de un record JSONL arbitrario. Reemplaza los nombres
de campo hardcodeados en `rawRecord` struct.

## Alcance

En `internal/input_config/selector.go`:

```go
// SelectField extrae un valor de un record JSON usando un JSONPath simple.
// Soporta paths como: "$.role", "$.message.role", "$.uuid"
func SelectField(record map[string]interface{}, path string) (interface{}, bool)

// SelectString extrae un string field (convierte tipos primitivos a string).
func SelectString(record map[string]interface{}, path string) (string, bool)
```

Subset de JSONPath a soportar (suficiente para los presets existentes):
- `$.field` — campo top-level
- `$.a.b` — campo anidado
- `$.a[0]` — primer elemento de array
- `$.a[*].b` — mapeo sobre array (retorna slice)

**No** es necesario soportar el spec completo de JSONPath — solo los patrones
usados en `claude.inputs.toml` y `pi.inputs.toml`.

## Criterios de aceptación

- `SelectField(record, "$.message.role")` extrae el campo correctamente de un fixture Claude
- `SelectField(record, "$.uuid")` extrae el uuid de un fixture Pi
- Paths inválidos retornan `("", false)` sin panic
- Test table-driven con fixtures de Claude y Pi
- `go test ./internal/input_config/...` pasa

## Notas

- Evaluar si una dependencia externa liviana (e.g., `github.com/tidwall/gjson`) es preferible
  a implementar el subset manualmente. Documentar la decisión.
