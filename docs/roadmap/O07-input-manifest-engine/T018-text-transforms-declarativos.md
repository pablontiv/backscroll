---
id: T018
tipo: task
estado: Completed
titulo: Text transforms declarativos
outcome: O07
dependencias: [T014]
---

# T018 — Text transforms declarativos

Implementar el pipeline de transformaciones de texto declarativas definidas en
`TextConfig`. Permite modificar el contenido de un mensaje antes de indexarlo
sin código hardcodeado.

## Alcance

En `internal/input_config/transform.go`:

```go
// ApplyTransforms aplica la lista de transforms al texto de entrada en orden.
func ApplyTransforms(transforms []Transform, text string) (string, error)
```

Transforms a implementar:
- `remove`: elimina ocurrencias de un patrón (regex si contiene metacaracteres, substring exacto si no)
- `trim`: elimina whitespace al inicio y al final
- `join`: colapsa múltiples líneas/espacios en uno (con separador configurable via `with`)
- `drop_empty`: si el resultado es vacío (solo whitespace), descarta el registro

## Criterios de aceptación

- Test table-driven con cada tipo de transform
- `remove` con regex inválido retorna error descriptivo
- `drop_empty` retorna `("", ErrDropped)` para texto vacío post-transformación
- Transforms se aplican en el orden definido en la lista
- `go test ./internal/input_config/...` pasa

## Referencias

- `TextConfig` / `apply_transforms()` en `src/input_config.rs` (v0 branch)
- `internal/sync/noise.go` — filtros de contenido actuales (referencia de comportamiento)
