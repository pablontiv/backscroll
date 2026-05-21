---
estado: Completed
tipo: task
---
# T001: Add picokit dependency to backscroll

**Outcome**: [O15 Integrate picokit as a dependency](README.md)
**Contribuye a**: picokit disponible como dependencia importable para T002-T006.

## Preserva

- INV1: `go build ./...` pasa tras agregar la dependencia.
  - Verificar: `go build ./...` desde `/home/shared/backscroll`.
- INV2: `go test ./...` pasa sin regresiones.
  - Verificar: `go test ./... -race -count=1`.

## Contexto

picokit v0.1.1 es el módulo `github.com/pablontiv/picokit`. Esta task solo agrega la dependencia; T002-T006 hacen las migraciones reales.

## Alcance

**In**:
1. `go get github.com/pablontiv/picokit@v0.1.1` desde `/home/shared/backscroll`.
2. `go mod tidy`.

**Out**:
- No modificar ningún `.go` de código fuente.
- No borrar paquetes internos aún.

## Estado inicial esperado

- `grep picokit /home/shared/backscroll/go.mod` retorna vacío.
- picokit v0.1.1 resuelve vía `go get`.

## Criterios de Aceptación

- `grep "pablontiv/picokit" /home/shared/backscroll/go.mod` muestra `v0.1.1` o superior.
- `go build ./...` pasa.
- `go test ./... -race -count=1` pasa.
