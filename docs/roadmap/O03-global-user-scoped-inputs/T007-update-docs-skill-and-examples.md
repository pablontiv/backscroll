---
estado: Completed
tipo: task
---
# T007: Update docs, skill and examples

**Outcome**: [O03 Global user-scoped inputs](README.md)
**Contribuye a**: CE5

[[blocked_by:./T001-implement-os-aware-global-input-loader.md]]
[[blocked_by:./T003-add-shipped-claude-and-pi-input-presets.md]]

## Preserva

- INV3: JMESPath/plugins siguen fuera del MVP.
  - Verificar: docs no prometen scripts ejecutables ni JMESPath como dependencia actual.
- INV4: `backscroll.toml` sigue siendo app config.
  - Verificar: docs no presentan `backscroll.toml`, `session_dirs` ni `[sources]` como rutas canónicas de ingesta.

## Contexto

La documentación viva y la skill aún mencionan manifests cwd-locales (`*.inputs.toml`, `backscroll.inputs.d`), `sync --path` y flags legacy como `--include-agents`. O03 cambia el modelo canónico a user config OS-aware y limpia parsers legacy. La documentación debe decir una sola cosa consistente para CLI, Pi/Claude skill e install.

## Alcance

**In**:
1. Actualizar `README.md` quick start, CLI examples y secciones de configuración para instalar/copiar presets y usar `<config_dir>/backscroll/inputs/*.inputs.toml`.
2. Actualizar `docs/input-contract.md` para declarar la ubicación canónica global/user-scoped y `BACKSCROLL_CONFIG_DIR`.
3. Actualizar `docs/configuration.md`, `docs/sync.md`, `docs/read.md` si aplica, y `backscroll.toml.example`.
4. Actualizar `.claude/skills/backscroll/SKILL.md` para que agentes usen `backscroll inputs validate/list/test` contra manifests globales instalados.
5. Remover o marcar como histórico cualquier mención actual a `./*.inputs.toml`, `backscroll.inputs.d`, `backscroll.inputs.toml`, `sync --path`, `--include-agents`, `session_dirs` como ingesta canónica o fallback Claude implícito.
6. Documentar rutas OS-aware: Linux/XDG, macOS Application Support, Windows `%APPDATA%`, y override `BACKSCROLL_CONFIG_DIR`.
7. Documentar que los presets shipped no se sobrescriben por defecto durante install.

**Out**:
- Cambiar roadmap histórico completado salvo que se necesite una nota de superseded.
- Documentar features no implementadas.
- Crear nuevos formatos de manifest.

## Estado inicial esperado

- T001/T003 definen loader y presets reales.
- La skill Backscroll está instalada a user scope por hooks, pero source vive en `.claude/skills/backscroll/SKILL.md`.

## Criterios de Aceptación

- Grep en docs/README/skill no encuentra menciones canónicas actuales a `./*.inputs.toml`, `backscroll.inputs.d`, `backscroll.inputs.toml`, `sync --path` ni `--include-agents`.
- README explica instalación de binary + input presets.
- Skill Backscroll indica revisar `backscroll inputs validate/list/test` y la ruta OS-aware global.
- Docs distinguen claramente app config (`backscroll.toml`) de input config global.

## Fuente de verdad

- `README.md`
- `docs/input-contract.md`
- `docs/configuration.md`
- `docs/sync.md`
- `docs/read.md`
- `backscroll.toml.example`
- `.claude/skills/backscroll/SKILL.md`
- `inputs/claude.inputs.toml`
- `inputs/pi.inputs.toml`
