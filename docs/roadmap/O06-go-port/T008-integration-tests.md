---
estado: Completed
tipo: task
---
# T008: Tests de integración y coverage gate

**Outcome**: [Port a Go](README.md)

## Contexto

Port de los tests de integración CLI actuales (`tests/cli.rs`, 3.459 líneas) a Go usando stdlib testing + subprocess o invocación directa de `run()`. Coverage gate ≥ 85% requerido por CI (go-ci.yml de crossbeam).

## Alcance

**In**:
1. Tests de integración CLI equivalentes a los actuales: sync con fixtures, search con filtros, read, resume, list, status, validate.
2. Tests de `decisions` con fixtures existentes (usando snapshots golden files donde corresponda).
3. Install script tests: adaptar `tests/test-install.sh` para el binary Go.
4. Coverage ≥ 85% global (`go test -cover ./...`).
5. CI verde en `main`: go-ci.yml + go-release.yml producen binary publicable.

**Out**:
1. Port 1:1 de cada test Rust — algunos pueden simplificarse si el comportamiento es idéntico.
2. Tests de embeddings (eliminados permanentemente).

## Criterios de Aceptación

- `go test ./...` pasa localmente con los fixtures existentes.
- Coverage report muestra ≥ 85% global.
- CI en `main` completa sin errores y produce release assets para las 5 plataformas (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64).
- `backscroll --version` en el binary release muestra la versión correcta inyectada por goreleaser.
