---
estado: Specified
tipo: task
---
# T002: Crear preset inputs/opencode.inputs.toml

**Outcome**: [O14 OpenCode Reader](README.md)
**Contribuye a**: el reader es activable sin configuración manual extra

[[blocked_by:./T001-fix-reader-schema.md]]

## Preserva

- INV1: Los presets existentes (`claude.inputs.toml`, `decisions.inputs.toml`) no se modifican
  - Verificar: `git diff --name-only` no incluye archivos existentes en `inputs/`

## Contexto

Backscroll distribuye presets de input en `inputs/*.inputs.toml`. Los usuarios los instalan con `cp inputs/opencode.inputs.toml ~/.config/backscroll/inputs/` y luego `backscroll sync`.

La DB de OpenCode en Linux vive en `~/.local/share/opencode/opencode.db` (XDG data dir, path hardcoded en el binario de anomalyco/opencode). El reader se selecciona por `decode.format = "opencode"`.

El preset debe tener `active = false` para que no afecte a usuarios que no tienen OpenCode instalado.

## Alcance

**In**:
1. Crear `inputs/opencode.inputs.toml` con la estructura estándar de preset
2. Roots apunta a `~/.local/share/opencode`, include = `["opencode.db"]`
3. `active = false`; comentario con instrucciones de instalación
4. `decode.format = "opencode"`
5. `source = "session"` para que los resultados aparezcan en búsquedas de sesiones

**Out**:
- No instalar el preset en `~/.config/backscroll/inputs/` (eso lo hace el usuario)
- No modificar `backscroll.toml` ni ningún archivo de config del sistema

## Estado inicial esperado

- No existe `inputs/opencode.inputs.toml`
- Existe `inputs/claude.inputs.toml` como referencia de estructura

## Criterios de Aceptación

- AC1: Archivo `inputs/opencode.inputs.toml` creado con `version = 1`, `active = false`, `decode.format = "opencode"`
- AC2: `roots = ["~/.local/share/opencode"]`, `include = ["opencode.db"]`
- AC3: Tiene comentario explicando cómo instalar y activar
- AC4: `backscroll inputs list --json` (después de instalar el preset) lista la entrada `opencode` como inactiva

## Fuente de verdad

- `inputs/claude.inputs.toml` (referencia de estructura)
- `internal/input_config/types.go` (campos soportados en InputDefinition)
