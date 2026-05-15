---
estado: Specified
tipo: task
---
# T017: Actualizar SKILL.md para incluir todos los presets en el comando de instalación

**Contribuye a**: El skill de Backscroll referencia todos los presets de instalación

## Preserva

- INV1: El skill sigue siendo invocable correctamente
  - Verificar: `backscroll status` dentro de una sesión que use el skill

## Contexto

`.claude/skills/backscroll/SKILL.md` tiene dos problemas:

1. **Frontmatter `description`**: dice "Claude/Pi sessions" — excluye OpenCode, lo que puede hacer que el skill no se active cuando el usuario pregunta por sesiones de OpenCode.

2. **Comando `cp` de instalación manual** (sección "Preflight"): copia solo `claude.inputs.toml` y `decisions.inputs.toml`, omitiendo `pi.inputs.toml` y `opencode.inputs.toml`. Un usuario que instale manualmente siguiendo ese snippet quedaría con presets incompletos.

El archivo fuente está en el repo en `.claude/skills/backscroll/SKILL.md`. Hay una copia distribuida en `~/.claude/skills/backscroll/` que se sincroniza vía pre-push hook — basta editar el archivo del repo.

## Alcance

**In**:
1. `description` en frontmatter: `"Claude/Pi sessions"` → `"Claude, Pi, or OpenCode sessions"`
2. Comando cp en sección Preflight: agregar `inputs/pi.inputs.toml inputs/opencode.inputs.toml`

**Out**:
- No modificar la lógica de invocación del skill
- No modificar `~/.claude/skills/backscroll/` directamente (se sincroniza en el push)

## Estado inicial esperado

- `grep "Claude/Pi" .claude/skills/backscroll/SKILL.md` encuentra match en description
- `grep "cp -n" .claude/skills/backscroll/SKILL.md` muestra solo 2 presets

## Criterios de Aceptación

- `grep "description:" .claude/skills/backscroll/SKILL.md` contiene "OpenCode"
- `grep "cp -n" .claude/skills/backscroll/SKILL.md` incluye `pi.inputs.toml` y `opencode.inputs.toml`
- `just check` pasa

## Fuente de verdad

- `.claude/skills/backscroll/SKILL.md` — archivo a modificar
