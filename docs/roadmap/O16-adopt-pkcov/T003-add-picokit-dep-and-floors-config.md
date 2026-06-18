---
estado: Completed
tipo: task
---
# T003: Agregar dep picokit + crear .coverage-floors.toml

**Outcome**: [O16 Adoptar pkcov de picokit](README.md)
**Contribuye a**: backscroll empieza a consumir el tooling compartido

[[blocked_by:./T001-raise-internal-storage-coverage.md]]
[[blocked_by:./T002-cover-or-remove-internal-models.md]]

## Preserva

- INV1 del outcome: threshold uniforme 85 — mismo número que el gate actual

## Contexto

Hoy backscroll no importa picokit. Esta task lo agrega como dependencia para usar `go run github.com/pablontiv/picokit/cmd/pkcov` desde recipes. Además declara `.coverage-floors.toml` siguiendo el schema de coverage-spec v1.0.

Dependencia cross-repo: picokit `O03-coverage-tooling` debe estar Completed con tag publicado (no se expresa como blocked_by por estar en otro repo).

## Alcance

**In**:

1. `go.mod`: agregar `github.com/pablontiv/picokit` en la versión que incluye `coverage`/`pkcov`. `go mod tidy`.
2. Crear `/home/shared/harness/backscroll/.coverage-floors.toml`:
   ```toml
   default = 85
   packages = [
     # listar paquetes actuales del repo — generar con:
     # go list ./... | sed 's|^github.com/pablontiv/backscroll/||'
   ]
   ```
3. Verificación: `go run github.com/pablontiv/picokit/cmd/pkcov check` (sobre `coverage.out` post-tests) exit 0.

**Out**:
- No modificar Justfile aún (T004).
- No tocar pre-push aún (T005).

## Estado inicial esperado

- T001 y T002 completadas: `internal/storage` ≥85, `internal/models` ≥85 o borrado.
- `go.mod` sin `github.com/pablontiv/picokit`.
- No existe `.coverage-floors.toml`.

## Criterios de Aceptación

- `go.mod` incluye `github.com/pablontiv/picokit` resoluble.
- `.coverage-floors.toml` existe con `default = 85` y la lista completa de paquetes vivos del repo.
- `go test ./... -coverprofile=/tmp/cov.out && go run github.com/pablontiv/picokit/cmd/pkcov check --profile /tmp/cov.out --floors .coverage-floors.toml` exit 0.
- `golangci-lint run` sin issues nuevos.

## Fuente de verdad

- `/home/shared/harness/backscroll/go.mod`
- `/home/shared/harness/backscroll/.coverage-floors.toml` (nuevo)
- `/home/shared/picokit/coverage/` (consumido)
