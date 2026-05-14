---
estado: Completed
tipo: task
---
# T008: Fix remaining CI failures (lint, tidy, coverage)

**Contribuye a**: make all CI jobs pass — Lint, Tidy, and Test (coverage) are still failing after T002/T003.

## Criterios de Aceptación

- `just check` passes locally.
- `go mod tidy` produces no diff.
- `go test -race -coverprofile=coverage.out ./... && scripts/check-coverage.sh` exits 0 (≥85% with -race).
- CI run shows all jobs green (verified with Monitor).

## Scope

### 1. Lint — 17 remaining errcheck violations

- `cmd/backscroll/main_test.go:22,25,27` — `os.Setenv`/`os.Unsetenv` unchecked; use `_ =`
- `cmd/backscroll/projects.go:113` — `fmt.Fprintln(stdout, alias)` unchecked; use `_, _ =`
- `internal/output/output.go:66,69,72,75,77` — conditional `fmt.Fprintf` calls unchecked; use `_, _ =`
- `internal/output/output.go:90–104` — `writeRobot` `fmt.Fprintf` calls unchecked; use `_, _ =`
- `internal/sources/sources_test.go:194,341` — `os.RemoveAll` in defer unchecked; use `defer func() { _ = os.RemoveAll(...) }()`
- `internal/storage/records.go:75` — `defer rows.Close()` unchecked; use `defer func() { _ = rows.Close() }()`
- `internal/storage/search.go:144,205` — `defer rows.Close()` unchecked; same pattern
- `internal/storage/storage_test.go:32,33,69,76,573` — `db.Close`/`os.Remove` unchecked; use `_ =`

### 2. Tidy — go.mod direct deps marked indirect

Run `go mod tidy` to move `cobra`, `go-toml/v2`, `modernc.org/sqlite` to the direct `require` block
and add `github.com/google/pprof` to `go.sum`.

### 3. Coverage — 84.1% with -race < 85% threshold

CI runs `go test ./... -race`. With -race, `internal/config` drops to 80.3% and `cmd/backscroll`
to 83.0%. Add targeted tests to bring aggregate to ≥85%.

## Fuentes de verdad

- `cmd/backscroll/main_test.go`
- `cmd/backscroll/projects.go`
- `internal/output/output.go`
- `internal/sources/sources_test.go`
- `internal/storage/records.go`
- `internal/storage/search.go`
- `internal/storage/storage_test.go`
- `go.mod`, `go.sum`
- `internal/config/`
