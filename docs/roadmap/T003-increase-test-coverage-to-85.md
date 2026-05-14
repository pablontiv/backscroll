---
estado: Completed
tipo: task
---
# T003: Increase test coverage to ≥85%

**Contribuye a**: make CI `ci / Test` coverage gate pass by bringing aggregate test coverage from 84.1% to ≥85%.

## Criterios de Aceptación

- `scripts/check-coverage.sh` exits 0 (aggregate ≥85%).
- CI `ci / Test` coverage check passes on push to main.
- No existing tests are removed or weakened.

## Análisis

Current per-package coverage (2026-05-13):

| Package | Coverage |
|---------|----------|
| `internal/storage` | 81.2% |
| `cmd/backscroll` | 84.5% |
| `internal/sync` | 85.1% |
| `internal/config` | 85.2% |
| `internal/sources` | 93.2% |
| `internal/plans` | 94.4% |
| `internal/projects` | 95.5% |
| `internal/output` | 96.6% |
| `internal/diagnostics` | 100.0% |
| `internal/reader` | 100.0% |
| `internal/tagging` | 100.0% |

`internal/storage` at 81.2% is the primary gap pulling the aggregate below threshold.

## Scope

- Identify uncovered code paths in `internal/storage` (use `go test -coverprofile` + `go tool cover -html`).
- Add targeted unit tests for uncovered storage functions/branches.
- If needed, add tests in `cmd/backscroll` to cover uncovered CLI code paths.
- Do not introduce tests that depend on external state or require network access.

## Fuentes de verdad

- `internal/storage/`
- `cmd/backscroll/`
- `scripts/check-coverage.sh`
