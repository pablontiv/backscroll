---
estado: Completed
tipo: task
---
# T004: Reemplazar scripts/check-coverage.sh por pkcov

**Outcome**: [O16 Adoptar pkcov de picokit](README.md)
**Contribuye a**: deduplicar el tooling; backscroll usa la misma implementación que los otros repos

[[blocked_by:./T003-add-picokit-dep-and-floors-config.md]]

## Preserva

- INV1 del outcome: threshold uniforme 85
  - Verificar: gate de CI sigue passing post-swap

## Contexto

`/home/shared/backscroll/scripts/check-coverage.sh` (17 líneas bash) valida sólo el total. Con T001-T003 completos, el repo está listo para adoptar pkcov que agrega per-package floors gratis (mismo total, granularidad mayor).

Justfile actual: revisar si existe `coverage`/`coverage-check` recipes. Si no, crear siguiendo el patrón rootline. Si sí, actualizar para invocar pkcov.

## Alcance

**In**:

1. Editar/crear `Justfile` recipes:
   - `coverage`: `go test ./... -coverprofile=coverage.out` + `go run github.com/pablontiv/picokit/cmd/pkcov report`
   - `coverage-check`: anterior + `go run github.com/pablontiv/picokit/cmd/pkcov check`
2. Borrar `scripts/check-coverage.sh`.
3. Si el CI `ci.yml` invoca `scripts/check-coverage.sh` directamente, actualizar para invocar `just coverage-check` o el `pkcov` directamente (consistente con cómo lo usa rootline).
4. Verificar localmente: `just coverage-check` exit 0.

**Out**:
- No tocar pre-push hook (T005).
- No cambiar threshold ni floors.toml.

## Estado inicial esperado

- T003 completada: `.coverage-floors.toml` existe + dep picokit en `go.mod`.
- `scripts/check-coverage.sh` existe y CI lo invoca.

## Criterios de Aceptación

- `Justfile` tiene recipes `coverage` y `coverage-check` invocando `pkcov`.
- `scripts/check-coverage.sh` no existe.
- `just coverage-check` exit 0 (total ≥85, todos los paquetes ≥85).
- Si CI workflow se modificó, gh action verde post-push.
- `go test ./... -race` verde.

## Fuente de verdad

- `/home/shared/backscroll/Justfile`
- `/home/shared/backscroll/scripts/check-coverage.sh` (a borrar)
- `/home/shared/backscroll/.github/workflows/ci.yml`
- `/home/shared/rootline/Justfile` — patrón a imitar
