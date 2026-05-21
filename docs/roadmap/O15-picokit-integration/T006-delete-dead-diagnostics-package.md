---
estado: Completed
tipo: task
---
# T006: Delete dead internal/diagnostics package

**Outcome**: [O15 Integrate picokit as a dependency](README.md)
**Contribuye a**: backscroll elimina código muerto sin callers; si en el futuro necesita `Error`/`Wrap`, usará `picokit/diag` directamente.

## Preserva

- INV1: `go build ./...` pasa tras el borrado (confirma que no hay callers).
  - Verificar: `go build ./...` después de `rm -rf internal/diagnostics/`.

## Contexto

`internal/diagnostics/diagnostics.go` define `type Error`, `New`, `Wrap` pero **cero callsites** en el árbol (verificado: `grep -rn "backscroll/internal/diagnostics" --include="*.go"` retorna vacío).

Mantener el paquete sería deuda silenciosa. picokit/diag ofrece la misma funcionalidad (más completa) si en algún momento se necesita.

## Alcance

**In**:
1. Borrar `internal/diagnostics/` completo (`diagnostics.go`, `diagnostics_test.go`).

**Out**:
- No reemplazar por nada (no hay callers).

## Estado inicial esperado

- `internal/diagnostics/` existe.
- `grep -rn "backscroll/internal/diagnostics" /home/shared/backscroll --include="*.go"` retorna vacío.

## Criterios de Aceptación

- `ls /home/shared/backscroll/internal/diagnostics/` falla (directorio borrado).
- `grep -rn "backscroll/internal/diagnostics" /home/shared/backscroll --include="*.go"` sigue retornando vacío.
- `go build ./...` pasa.
- `go test ./... -race -count=1` pasa.
