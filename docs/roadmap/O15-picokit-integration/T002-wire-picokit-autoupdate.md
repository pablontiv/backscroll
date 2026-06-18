---
estado: Completed
tipo: task
---
# T002: Wire picokit/autoupdate into backscroll CLI

**Outcome**: [O15 Integrate picokit as a dependency](README.md)
**Contribuye a**: backscroll gana self-update vía staged async pattern.

[[blocked_by:./T001-add-picokit-dependency.md]]

## Preserva

- INV1: `BACKSCROLL_AUTOUPDATE_DISABLE=1 ./backscroll --version` no hace requests HTTP.
- INV2: `version == "dev"` no dispara update.
- INV3: `FetchAndStage` corre en goroutine y no bloquea el exit del CLI.
- INV4: La integridad del binario staged se verifica vía SHA-256 contra `checksums.txt` del release (cubierto por picokit, no duplicar tests aquí).

## Contexto

`cmd/backscroll/main.go:11` define `var version = "dev"`, inyectado vía `-X main.version={{.Version}}` en goreleaser. goreleaser produce `backscroll_{version}_{os}_{arch}.tar.gz` + `checksums.txt`, compatible con picokit/autoupdate. install.sh ya consume el patrón.

Wiring propuesto en `cmd/backscroll/main.go` (tras `var version = "dev"` y antes de la cobra root command):
```go
u := autoupdate.New("pablontiv/backscroll", "backscroll", "BACKSCROLL_AUTOUPDATE_DISABLE")
_ = u.ApplyStagedIfAvailable()
go u.FetchAndStage(version) //nolint:errcheck
```

Env disable convention: `BACKSCROLL_AUTOUPDATE_DISABLE` (consistente con `BACKSCROLL_*` ya presentes: `INSTALL_DIR`, `CONFIG_DIR`, `DATABASE_PATH`).

## Alcance

**In**:
1. Agregar import `github.com/pablontiv/picokit/autoupdate` en `cmd/backscroll/main.go`.
2. Wirear `ApplyStagedIfAvailable` (sync) y `FetchAndStage` (goroutine).
3. Agregar 4 tests en `cmd/backscroll/main_test.go`:
   - `TestMain_AutoupdateConstructorParams`
   - `TestMain_AutoupdateSkipsOnEnv`
   - `TestMain_AutoupdateSkipsOnDevVersion`
   - `TestMain_AutoupdateFetchRunsInGoroutine`
4. Smoke script `scripts/test-autoupdate-smoke.sh`.

**Out**:
- No tocar `internal/output/`, `internal/sync/`, `internal/input_config/`, `internal/diagnostics/` (otras tasks).
- No modificar `.goreleaser.yml`.

## Estado inicial esperado

- T001 completada — picokit en go.mod.
- `cmd/backscroll/main.go:11` tiene `var version = "dev"`.
- goreleaser/install.sh ya producen el asset compatible.

## Criterios de Aceptación

- `grep -n "picokit/autoupdate" /home/shared/harness/backscroll/cmd/backscroll/main.go` muestra el import.
- Los 4 tests nuevos pasan.
- `BACKSCROLL_AUTOUPDATE_DISABLE=1 ./backscroll --version` termina <100ms sin HTTP.
- `go build ./...` pasa.
- `go test ./... -race -count=1` pasa.
- `scripts/check-coverage.sh` pasa con threshold ≥85%.
