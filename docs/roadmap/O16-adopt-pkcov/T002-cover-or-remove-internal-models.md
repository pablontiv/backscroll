---
estado: Specified
tipo: task
---
# T002: Cubrir o borrar internal/models

**Outcome**: [O16 Adoptar pkcov de picokit](README.md)
**Contribuye a**: precondición para activar pkcov; aplica política de dead code si corresponde (INV2)

## Preserva

- INV2 del outcome: no se relaja el contrato — o el paquete se cubre o se borra (no se ignora)
  - Verificar: o `go test ./internal/models/ -cover` ≥85, o el paquete no existe

## Contexto

`internal/models` aparece sin tests en el análisis (`go test ./... -cover` no reporta coverage para el paquete). Hay dos caminos legítimos según coverage-spec v1.0 sección 6:

1. **Cubrir**: si el paquete está en uso, agregar tests hasta ≥85.
2. **Borrar**: si el paquete está deprecado o no se importa desde código vivo, eliminarlo (sigue la política de dead code).

Esta task evalúa cuál camino aplica y ejecuta el elegido.

## Alcance

**In**:

1. Verificar uso: `grep -r "backscroll/internal/models" --include="*.go" .` para listar callers.
2. **Si tiene callers vivos**: agregar tests a `internal/models/*_test.go` hasta ≥85.
3. **Si no tiene callers vivos**: borrar `internal/models/` completo, verificar que `go build ./...` y `go test ./...` siguen verdes.
4. Documentar la decisión en el commit message.

**Out**:
- No refactor de lógica.
- No tocar otros paquetes.

## Estado inicial esperado

- `go test ./internal/models/ -cover` no reporta cobertura (sin tests) o muestra paquete sin source statements.

## Criterios de Aceptación

- Una de las dos condiciones se cumple:
  - `go test ./internal/models/ -cover` ≥85.0%, o
  - `internal/models/` no existe (borrado).
- `go build ./...` verde.
- `go test ./... -race` verde.
- Commit message documenta cuál camino se tomó y por qué (e.g. "no callers found, removed per coverage-spec dead-code policy").

## Fuente de verdad

- `/home/shared/backscroll/internal/models/`
- `/home/shared/backscroll/` — buscar callers
