---
estado: Completed
tipo: task
---
# T002: Fix golangci-lint violations

**Contribuye a**: make CI `ci / Lint` job pass by resolving all errcheck, staticcheck, and unused violations reported by golangci-lint v2.10.1.

## Criterios de Aceptación

- `just check` (gofmt + go vet) passes.
- `golangci-lint run ./...` returns 0 issues locally.
- CI `ci / Lint` job passes on push to main.

## Scope

### errcheck — production code

- `internal/storage/storage.go:25,31,53` — `db.Close` return values unchecked; use `_ =` or log error.
- `internal/storage/migrations.go:46` — `tx.Rollback` unchecked in defer; use `defer func() { _ = tx.Rollback() }()` pattern.
- `internal/storage/queries.go:81,171,196,201,282,297` — `rows.Close`, `tagRows.Close`, `tx.Rollback` unchecked; same defer pattern.
- `internal/storage/sync.go:40` — `tx.Rollback` unchecked; same defer pattern.
- `internal/sync/sync.go:26,86` — `file.Close` unchecked in defer; use `defer func() { _ = f.Close() }()`.
- `internal/output/output.go:62,63,64` — `fmt.Fprintf` to io.Writer unchecked; use `_ = fmt.Fprintf(...)`.
- `cmd/backscroll/decisions.go:352,801,1019` — `fmt.Fprintln` unchecked; use `_ = fmt.Fprintln(...)`.
- `cmd/backscroll/purge.go:30` — `cmd.MarkFlagRequired` unchecked; use `_ = cmd.MarkFlagRequired(...)`.

### errcheck — test code

- `internal/config/config_test.go:15,17,20,22,27,33,34,59,77` — `os.Setenv`, `os.Unsetenv`, `os.Chdir` unchecked; use `require.NoError` or explicit check.
- `internal/plans/plans_test.go:61,66,103,139,164` — `os.Remove`, `tmpfile.Close`, `os.RemoveAll` unchecked.
- `internal/sources/sources_test.go:61,66,156,161` — `os.Remove`, `tmpfile.Close` unchecked.
- `internal/storage/storage_test.go:559,566,573` — `db1.Close`, `db2.Close`, `db3.Close` unchecked.
- `internal/storage/unit_test.go:134` — `rodb.Close` unchecked.

### staticcheck

- `internal/sync/sync.go:151` — S1008: replace `if content == "" { return true }; return false` with `return content == ""`.
- `internal/sync/sync.go:313` — S1017: replace conditional `strings.TrimPrefix` with unconditional call.

### unused

- `cmd/backscroll/decisions_test.go:219` — `exEntry` type is unused; remove it.
- `internal/plans/plans_test.go:153` — `hasExtension` function is unused; remove it.

## Fuentes de verdad

- `internal/storage/storage.go`
- `internal/storage/migrations.go`
- `internal/storage/queries.go`
- `internal/storage/sync.go`
- `internal/sync/sync.go`
- `internal/output/output.go`
- `internal/config/config_test.go`
- `internal/plans/plans_test.go`
- `internal/sources/sources_test.go`
- `internal/storage/storage_test.go`
- `internal/storage/unit_test.go`
- `cmd/backscroll/decisions.go`
- `cmd/backscroll/decisions_test.go`
- `cmd/backscroll/purge.go`
