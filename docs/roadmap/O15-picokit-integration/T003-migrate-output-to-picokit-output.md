---
estado: Specified
tipo: task
---
# T003: Migrate internal/output to picokit/output (Format string→int refactor)

**Outcome**: [O15 Integrate picokit as a dependency](README.md)
**Contribuye a**: backscroll consume `picokit/output` upstream, elimina su duplicado local que difiere solo en el tipo del enum Format.

[[blocked_by:./T001-add-picokit-dependency.md]]

## Preserva

- INV1: Los outputs CLI de `search` y `resume` permanecen byte-for-byte idénticos en los 3 formatos (text, json, robot).
  - Verificar: golden tests con fixtures actuales pasan sin tocar.
- INV2: El contrato JSON consumido por LLMs vía `--format=robot` no cambia.
  - Verificar: `TestSearch_Output_FormatRobot` con golden fixture preservado.
- INV3: El comportamiento de `--max-tokens` (token limiting) se mantiene.
  - Verificar: `TestSearch_Output_RespectsTokenLimit` con `--max-tokens=100`.

## Contexto

`internal/output/output.go` define `type Format string` con `FormatText`/`FormatJSON`/`FormatRobot`, el `Formatter` struct, `NewFormatter`, `WriteResults`, `WriteJSON`. `picokit/output` define `type Format int` con constantes integer, `Formatter`, `NewFormatter`, `WriteJSON`, `WriteLines`, `TokenCount`.

Diferencias críticas:
- `Format string` (backscroll) vs `Format int` (picokit) — refactor de todos los callsites.
- `WriteResults(stdout, []models.SearchResult)` **no existe en picokit/output** — los callers deben re-emitir como `WriteJSON(stdout, results)` (formato JSON) o `WriteLines(stdout, marshalToLines(results))` (text/robot).

Callsites a actualizar (8):
- `cmd/backscroll/search.go:156,158,160,164,165`
- `cmd/backscroll/resume.go:112,116,117`

Este es el cambio más invasivo de O15. Riesgo principal: divergencia byte-for-byte en `--format=robot` (contrato consumido por LLMs).

## Alcance

**In**:
1. Reescribir 8 callsites para usar `picokit/output.Format` (int) y `NewFormatter`.
2. Implementar adapter local `marshalToLines(results []models.SearchResult) []string` que produzca exactamente el mismo output que el `WriteResults` actual para text/robot.
3. Borrar `internal/output/` entero (incluye `output_test.go`).
4. Agregar tests en `cmd/backscroll/search_test.go` y `resume_test.go`:
   - `TestSearch_Output_FormatText` (golden)
   - `TestSearch_Output_FormatJSON` (golden)
   - `TestSearch_Output_FormatRobot` (golden, contrato LLM)
   - `TestSearch_Output_RespectsTokenLimit`
   - `TestResume_Output_FormatText` (golden)
   - `TestResume_Output_FormatJSON` (golden)
   - `TestSearch_Output_AdapterRoutesByFormat` (verifica el switch sobre Format)

**Out**:
- No cambiar la API pública de los flags `--format` y `--max-tokens`.
- No introducir nuevos formatos.

## Estado inicial esperado

- T001 completada — picokit en go.mod.
- `internal/output/output.go` existe con `Format string`.
- 8 callsites en `cmd/backscroll/{search,resume}.go`.

## Criterios de Aceptación

- `grep -rn "backscroll/internal/output" /home/shared/backscroll --include="*.go"` retorna vacío.
- `grep -rn "pablontiv/picokit/output" /home/shared/backscroll --include="*.go"` retorna ≥1 import.
- `ls /home/shared/backscroll/internal/output/` falla (directorio borrado).
- Los 7 tests nuevos pasan.
- Golden outputs CLI byte-for-byte idénticos contra fixtures previos en `tests/fixtures/`.
- `go build ./...` pasa.
- `go test ./... -race -count=1` pasa.
- `scripts/check-coverage.sh` pasa con threshold ≥85% (`cmd/backscroll` mantiene branch coverage en el switch de formatos).
