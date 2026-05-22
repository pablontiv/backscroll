---
estado: Specified
tipo: task
---
# T021: Fix `TestResumeOutputFormatRobot` flake — setup self-contained

**Outcome**: [O17 Sanitize CI local](README.md)
**Contribuye a**: eliminar el test flake recurrente que se manifiesta como "No relevant sessions found for: test" en CI pero pasa localmente.

## Contexto

`TestResumeOutputFormatRobot` (ubicado en `cmd/backscroll/resume_test.go` o similar) falla intermitente en CI con "No relevant sessions found for: test". Pasa localmente porque depende de sesiones reales en `~/.claude/projects/` o un seed que no existe en el runner.

Diagnóstico esperado:
- El test invoca el comando `resume` y espera resultados para query="test".
- En CI no hay sesiones indexadas con ese token → resultado vacío → falla.
- Localmente sí hay sesiones por accidente histórico → pasa.

Fix: hacer el test self-contained — crear fixture inline (write tempfile JSONL, sync sobre tempdir, query) y aislar via `t.Setenv` (`HOME`, `BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIRS`, `BACKSCROLL_CONFIG_DIR`).

## Alcance

**In**:
1. Localizar el test: `grep -rn "TestResumeOutputFormatRobot" cmd/ internal/`.
2. Modificar el setup para crear un fixture mínimo (≥1 sesión JSONL en `t.TempDir()` cuyo contenido matchee la query) y apuntar las variables relevantes al tempdir.
3. Verificar el test pasa offline 10 veces: `go test -run TestResumeOutputFormatRobot -count=10 -race`.

**Out**:
- No refactorizar el comando `resume` ni su output format.
- No tocar otros tests del paquete.

## Estado inicial esperado

- `TestResumeOutputFormatRobot` existe y falla intermitente en CI.
- `go test -run TestResumeOutputFormatRobot` pasa localmente al menos a veces.

## Criterios de Aceptación

- El test no depende de archivos fuera de `t.TempDir()` ni de variables de entorno preexistentes (verificable leyendo el código tras el fix).
- `go test ./cmd/backscroll/ -run TestResumeOutputFormatRobot -count=10 -race` pasa 10/10.
- En CI, el último run del workflow `ci.yml` en backscroll/main muestra el test en passed.

## Fuente de verdad

- `/home/shared/backscroll/cmd/backscroll/resume_test.go` (probable ubicación; confirmar)
- `/home/shared/backscroll/cmd/backscroll/resume.go`
