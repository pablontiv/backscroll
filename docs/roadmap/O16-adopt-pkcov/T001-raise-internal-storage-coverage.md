---
estado: Completed
tipo: task
---
# T001: Subir internal/storage a ≥85% coverage

**Outcome**: [O16 Adoptar pkcov de picokit](README.md)
**Contribuye a**: precondición para activar pkcov sin que bloquee push (INV2)

## Preserva

- INV2 del outcome: el pre-work no relaja el contrato — sube el paquete débil al estándar
  - Verificar: `go test ./internal/storage/ -cover` ≥85

## Contexto

`internal/storage` está hoy en 82.4% (medido `2026-05-21`). Activar el floor per-package con pkcov sin antes subir este paquete bloquearía todo push subsiguiente. La task identifica las ramas/funciones sin cubrir, agrega tests, y verifica que el paquete cruza el umbral.

Estrategia: ejecutar `go test ./internal/storage/ -coverprofile=/tmp/storage.out && go tool cover -func=/tmp/storage.out | awk '$3 < "85.0%"'` para identificar las funciones con menor cobertura, y escribir tests específicos para esas.

## Alcance

**In**:

1. Identificar funciones de `internal/storage` con cobertura <85%.
2. Agregar tests (en `internal/storage/*_test.go`) que cubran esas ramas.
3. Validar que el paquete entero cruza 85.

**Out**:
- No refactor de la lógica de `internal/storage` (sólo tests).
- No tocar otros paquetes en este task.
- No agregar dependencia a picokit (eso es T003).

## Estado inicial esperado

- `go test ./internal/storage/ -cover` reporta ~82.4%.

## Criterios de Aceptación

- `go test ./internal/storage/ -cover` ≥85.0%.
- `go test ./... -race` verde.
- Diff incluye sólo archivos `_test.go` (y posibles `testdata/`) en `internal/storage/`.
- `golangci-lint run` sin issues nuevos.

## Fuente de verdad

- `/home/shared/backscroll/internal/storage/` — paquete target
