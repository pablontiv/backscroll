---
estado: Specified
tipo: task
---
# T004: Migrate internal/sync.HashFile to picokit/hashfile.HashFile

**Outcome**: [O15 Integrate picokit as a dependency](README.md)
**Contribuye a**: backscroll consume su propia contribución a picokit (HashFile fue extraído desde `internal/sync.HashFile` durante O01 de picokit) en lugar de mantener la copia local.

[[blocked_by:./T001-add-picokit-dependency.md]]

## Preserva

- INV1: El hash SHA-256 producido es idéntico al actual (mismo contrato).
  - Verificar: tests existentes en `internal/readers/jsonl_reader_test.go` pasan sin tocar assertions.
- INV2: Errores con paths inexistentes/no accesibles tienen la misma semántica.
  - Verificar: comportamiento de error idéntico al actual.

## Contexto

`internal/sync/sync.go:80-94` define `HashFile(path) (string, error)`. Fue la fuente original de `picokit/hashfile.HashFile`. Hoy backscroll mantiene la copia local.

Callsites (4):
- `internal/readers/jsonl_reader.go:21,28`
- `internal/readers/jsonl_reader_test.go:26,136`

## Alcance

**In**:
1. Cambiar imports en `internal/readers/jsonl_reader.go` y `jsonl_reader_test.go`: `bsync "backscroll/internal/sync"` → `"github.com/pablontiv/picokit/hashfile"`.
2. Reemplazar `bsync.HashFile(path)` por `hashfile.HashFile(path)` en los 4 callsites.
3. Borrar `HashFile` de `internal/sync/sync.go:80-94` y su test asociado en `internal/sync/sync_test.go` (si existe).

**Out**:
- No tocar otras funciones de `internal/sync/` (solo `HashFile`).
- No introducir `picokit/hashfile.WriteAtomic` (no hay callsites en backscroll que lo necesiten hoy).

## Estado inicial esperado

- T001 completada — picokit en go.mod.
- `internal/sync/sync.go:80-94` tiene `HashFile`.
- 4 callsites referencian `bsync.HashFile`.

## Criterios de Aceptación

- `grep -n "HashFile" /home/shared/backscroll/internal/sync/sync.go` retorna 0 matches.
- `grep -rn "bsync.HashFile" /home/shared/backscroll --include="*.go"` retorna vacío.
- `grep -rn "hashfile.HashFile" /home/shared/backscroll --include="*.go"` retorna ≥2 imports.
- `go test ./internal/readers/... -race -count=1` pasa sin tocar assertions.
- `go build ./...` pasa.
- `scripts/check-coverage.sh` pasa con threshold ≥85%.
