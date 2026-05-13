---
estado: Completed
tipo: task
---
# T002: Scaffold de módulos core

**Outcome**: [Port a Go](README.md)

## Contexto

Tres módulos sin dependencias internas que sirven de base para todo lo demás: configuración, diagnósticos de error y formateo de output. Modelados según el patrón de roadmapctl (internal/diagnostics, internal/config).

## Alcance

**In**:
1. `internal/diagnostics` — structured error reporting equivalente a miette; sin dependencias externas.
2. `internal/config` — resolución de config: `backscroll.toml` local → `~/.config/backscroll/config.toml` → env vars (`BACKSCROLL_DATABASE_PATH`, `BACKSCROLL_SESSION_DIRS`) → defaults. Usando go-toml/v2.
3. `internal/output` — formateador de resultados: Text, JSON, Robot; token limiting aproximado.
4. Tests unitarios para los tres módulos.

**Out**:
1. Comandos CLI (van en T007).
2. Lectura de sesiones (va en T003).

## Criterios de Aceptación

- `go build ./internal/...` compila sin errores.
- Tests de config cubren: archivo local, archivo global, env vars, defaults, multi-path `session_dirs` con alias `session_dir`.
- Tests de output cubren: Text, JSON, Robot con token limiting.
- `go test ./internal/config/... ./internal/output/... ./internal/diagnostics/...` pasa.
