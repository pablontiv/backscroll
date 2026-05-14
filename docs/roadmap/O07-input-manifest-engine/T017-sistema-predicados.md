---
id: T017
tipo: task
estado: Completed
titulo: Sistema de predicados (eq, ne, in, exists, missing)
outcome: O07
dependencias: [T014]
---

# T017 — Sistema de predicados (eq, ne, in, exists, missing)

Implementar el evaluador de predicados declarativos para filtrar registros durante
el parsing. Reemplaza el noise filtering hardcodeado en `internal/sync/noise.go`.

## Alcance

En `internal/input_config/predicate.go`:

```go
// EvalPredicate evalúa un predicado contra un valor extraído de un record.
// field ya fue extraído via JSONPath; value es el valor del campo.
func EvalPredicate(p Predicate, value interface{}) (bool, error)

// EvalPredicates evalúa una lista de predicados (AND semántica).
func EvalPredicates(predicates []Predicate, record map[string]interface{}) (bool, error)
```

Operadores a implementar:
- `eq`: igualdad exacta (string, number, bool)
- `ne`: desigualdad
- `in`: el valor está en la lista dada
- `exists`: el campo existe y no es null
- `missing`: el campo no existe o es null

## Criterios de aceptación

- Test table-driven con todos los operadores y tipos (string, number, bool, null)
- `exists` retorna true para valores vacíos string `""` (campo existe pero vacío)
- `missing` retorna true para campo ausente o null
- `in` soporta lista de strings y lista de numbers
- Error descriptivo para operador desconocido
- `go test ./internal/input_config/...` pasa

## Referencias

- `Predicate` / `eval_predicate()` en `src/input_config.rs` (v0 branch)
- `internal/sync/noise.go` — filtros hardcodeados actuales (referencia de comportamiento)
