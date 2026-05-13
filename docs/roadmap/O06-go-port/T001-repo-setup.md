---
estado: Specified
tipo: task
---
# T001: Setup del repo Go

**Outcome**: [Port a Go](README.md)

## Contexto

El repo vive en `master`. Hay que crear `v0` (Rust congelado), crear `main` limpio para el port Go, y configurar todo el scaffolding inicial: módulo Go, goreleaser, CI.

## Alcance

**In**:
1. Crear rama `v0` desde el HEAD actual de `master` y pushear a GitHub.
2. Cambiar el default branch de GitHub a `main`.
3. En `main`: eliminar todo el código Rust, inicializar `go mod init github.com/pablontiv/backscroll`.
4. Copiar `.goreleaser.yaml` de roadmapctl adaptado (`binary-name: backscroll`, `main: ./cmd/backscroll`, plataformas linux/darwin/windows amd64/arm64).
5. Copiar `ci.yml` de roadmapctl adaptado (go-ci.yml + gitleaks + go-release.yml, coverage-threshold 85).
6. Actualizar `CLAUDE.md` para reflejar el stack Go.
7. Commit inicial en `main` con estructura vacía compilable (`main.go` mínimo).

**Out**:
1. Migrar datos de la DB existente.
2. Modificar código Rust en `v0`.

## Criterios de Aceptación

- La rama `v0` existe en GitHub y su CI Rust pasa.
- La rama `main` es el default branch en GitHub.
- `go build ./...` compila sin errores desde `main`.
- `go test ./...` pasa (sin tests aún = OK).
- El CI en `main` corre go-ci.yml y go-release.yml desde crossbeam.
