---
estado: Specified
tipo: task
---
# T005: Adopt picokit/pathsec in internal/input_config/discover.go

**Outcome**: [O15 Integrate picokit as a dependency](README.md)
**Contribuye a**: backscroll endurece su superficie de input discovery contra symlink escapes y absolute path injection.

[[blocked_by:./T001-add-picokit-dependency.md]]

## Preserva

- INV1: Paths con symlinks que apuntan fuera del root son rechazados.
  - Verificar: `TestDiscover_RejectsSymlinkEscape` pasa.
- INV2: Paths absolutos fuera del root son rechazados.
  - Verificar: `TestDiscover_RejectsAbsolutePath` pasa.
- INV3: Paths relativos válidos dentro del root siguen funcionando.
  - Verificar: `TestDiscover_AcceptsValidNested` pasa.
- INV4: La expansión de tilde (`~`) sigue funcionando como hoy.
  - Verificar: `TestDiscover_AcceptsTildeExpansion` pasa.

## Contexto

`internal/input_config/discover.go:14-103` hace tilde expansion + `filepath.Abs` + `filepath.WalkDir` + `EvalSymlinks` sin guard anti-escape sistemático. Es la superficie más expuesta a inputs externos del usuario en backscroll (input presets, sources discovery).

`picokit/pathsec.ResolveInside(root, candidate)` valida que `candidate` no escape de `root`, rechaza absolutos, y resuelve symlinks de forma segura. No hace tilde expansion (picokit no asume layouts del usuario), así que la tilde expansion local se mantiene antes de pasar el path a `ResolveInside`.

## Alcance

**In**:
1. Agregar import `github.com/pablontiv/picokit/pathsec` en `internal/input_config/discover.go`.
2. Envolver path resolution post-glob (líneas 41-62) con `pathsec.ResolveInside(absRoot, candidate)`.
3. Propagar errores `pathsec.ErrPathEscape` y `pathsec.ErrAbsolutePath` como diagnostics de discovery.
4. Agregar 4 tests en `internal/input_config/discover_test.go`:
   - `TestDiscover_RejectsSymlinkEscape`
   - `TestDiscover_RejectsAbsolutePath`
   - `TestDiscover_AcceptsValidNested`
   - `TestDiscover_AcceptsTildeExpansion`

**Out**:
- No quitar la tilde expansion local.
- No introducir pathsec en otras superficies (separar en futuro outcome si necesario).

## Estado inicial esperado

- T001 completada — picokit en go.mod.
- `internal/input_config/discover.go:14-103` no usa pathsec.

## Criterios de Aceptación

- `grep -n "pathsec.ResolveInside" /home/shared/backscroll/internal/input_config/discover.go` retorna ≥1 match.
- Los 4 tests nuevos pasan.
- `go build ./...` pasa.
- `go test ./internal/input_config/... -race -count=1` pasa.
- `scripts/check-coverage.sh` pasa con threshold ≥85%.
