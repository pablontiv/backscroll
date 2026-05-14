---
id: T020
tipo: task
estado: Pending
titulo: Claude preset funcional en runtime
outcome: O07
dependencias: [T016, T017, T018, T019]
---

# T020 — Claude preset funcional en runtime

Verificar y ajustar `inputs/claude.inputs.toml` para que sea consumido correctamente
por el motor declarativo. Producir el mismo resultado que el parser hardcodeado actual.

## Alcance

- Instalar `inputs/claude.inputs.toml` en `~/.config/backscroll/inputs/` como parte
  del setup/instalación (documentar en README)
- Verificar que el preset cubre: discovery de `.jsonl` en `~/.claude/projects/`,
  exclusión de subagents, mapeo de campos Claude (role, uuid, timestamp, content)
- Ajustar el preset si hay discrepancias con el comportamiento actual
- Test de regresión: `backscroll sync` con preset Claude produce ≥N sesiones
  (N = cantidad actual en la DB de referencia)

## Criterios de aceptación

- `backscroll sync` con `claude.inputs.toml` activo indexa las mismas sesiones que hoy
- No hay regresión en los tests de integración existentes
- El preset está documentado en `inputs/README.md` o similar
- `go test ./...` pasa

## Notas

- Si el preset necesita ajustes, los cambios van al archivo TOML, no al código Go
- La paridad exacta de resultados se verifica comparando el count de sesiones antes/después
