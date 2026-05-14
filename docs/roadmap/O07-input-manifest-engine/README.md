---
id: O07
tipo: outcome
estado: Completed
titulo: Input Manifest Engine en Go
descripcion: Port del motor declarativo de inputs TOML (src/input_config.rs) al Go port. Permite definir fuentes de sesiones via *.inputs.toml en lugar de session_dirs hardcodeados.
---

# O07 — Input Manifest Engine en Go

Port completo de `src/input_config.rs` (~1000 LOC del branch v0) al Go port.
Introduce configuración declarativa de inputs via `*.inputs.toml` cargados desde
`~/.config/backscroll/inputs/`. Base de O08 (pipeline genérico) y O11 (subcomandos `inputs`).

## Tasks

- [T014](T014-definir-tipos-input-manifest.md) — Definir tipos Go para `*.inputs.toml`
- [T015](T015-loader-inputs-toml.md) — Loader de `~/.config/backscroll/inputs/`
- [T016](T016-glob-discovery-declarativo.md) — Glob discovery declarativo
- [T017](T017-sistema-predicados.md) — Sistema de predicados (eq, ne, in, exists, missing)
- [T018](T018-text-transforms-declarativos.md) — Text transforms declarativos
- [T019](T019-separar-app-config-input-config.md) — Separar app config de input config
- [T020](T020-claude-preset-runtime.md) — Claude preset funcional en runtime
- [T021](T021-pi-preset-runtime.md) — Pi preset funcional en runtime
- [T022](T022-subcomando-inputs.md) — Subcomando `inputs` (list, aliases, identify, test)

## Criterios de cierre

- `backscroll sync` usa inputs declarativos en lugar de `session_dirs` hardcodeado
- `backscroll inputs list` muestra inputs activos
- `session_dirs` en `backscroll.toml` sigue funcionando (backward compat)
- `go test ./...` pasa con coverage ≥85%
