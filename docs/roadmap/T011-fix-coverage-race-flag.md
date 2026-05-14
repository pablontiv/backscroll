---
estado: Completed
tipo: task
---
# T011: Fix CI coverage gap with -race flag

**Contribuye a**: all CI pipelines must pass (coverage gate ≥85%).

## Problem

Crossbeam `go-ci.yml` runs `go test ./... -race -coverprofile=coverage.out`, yielding 84.6% aggregate — below the 85% threshold. Local `scripts/check-coverage.sh` omits `-race`, so it passes locally (85.4%) but fails in CI.

Root cause: `cmd/backscroll` is at 83.3% with `-race` (vs 84.5% without) and `internal/storage` is consistently at 81.6%. The combined aggregate sits at 84.6%.

## Criterios de Aceptación

- `go test ./... -race -coverprofile=coverage.out` aggregate ≥ 85% (verified locally).
- `scripts/check-coverage.sh` still passes.
- All existing tests still pass with and without `-race`.
- No fabricated tests — all tests must exercise real code paths.

## Scope

Add targeted tests to close the gap:

1. `internal/storage` — currently 81.6%; identify uncovered functions and add tests.
2. `cmd/backscroll` — identify code paths missed under `-race` and add integration tests.

## Fuentes de verdad

- CI log: run 25837687036, job `ci / Test`, coverage 84.6%.
- `scripts/check-coverage.sh` — local gate script.
- `.github/workflows/ci.yml` → crossbeam `go-ci.yml@v1` uses `-race`.
