---
id: T019
tipo: task
estado: Pending
titulo: Separar app config de input config; backward compat session_dirs
outcome: O07
dependencias: [T015]
---

# T019 — Separar app config de input config; backward compat `session_dirs`

`backscroll.toml` es config de la aplicación (DB path, embedding config, sources).
`*.inputs.toml` en `~/.config/backscroll/inputs/` son la config de inputs.
Esta task implementa la separación limpia y la ruta de compatibilidad hacia atrás.

## Alcance

- `internal/config/config.go`: mantener `SessionDirs []string` pero marcarlo como
  deprecated en comentario; agregar `InputsDir string` override opcional
- `internal/input_config/compat.go`: función `SessionDirsToManifest(dirs []string) InputManifest`
  que genera un manifest JSONL implícito para los `session_dirs` configurados
- Al arrancar: si hay `*.inputs.toml` → usar manifests declarativos;
  si no hay manifests pero hay `session_dirs` → usar compat manifest generado
- `backscroll status` muestra "using declarative inputs" vs "using session_dirs (legacy)"

## Criterios de aceptación

- Config existente con `session_dirs` funciona sin cambios (backward compat total)
- Config nueva con `*.inputs.toml` usa manifests declarativos
- `backscroll status` indica el modo activo
- No se puede tener `session_dirs` Y manifests declarativos al mismo tiempo sin warning
- `go test ./...` pasa; no regresiones

## Notas de implementación

- El compat manifest usa `include = ["<dir>/**/*.jsonl"]` + `exclude = ["**/subagents/**"]`
  para replicar exactamente el comportamiento actual de `WalkSessionDirs()`
