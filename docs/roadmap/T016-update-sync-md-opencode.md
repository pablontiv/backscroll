---
estado: Specified
tipo: task
---
# T016: Actualizar docs/sync.md para incluir OpenCode

**Contribuye a**: sync.md sin referencias desactualizadas que omiten OpenCode

## Preserva

- INV1: `just check` pasa
  - Verificar: `just check`

## Contexto

`docs/sync.md` tiene dos menciones que excluyen OpenCode:

1. **Línea 113**: "This lets the shipped Claude and Pi presets both be active even when only one tool is installed" — OpenCode también está en los presets enviados (aunque inactive por defecto).

2. **Línea 117**: "Claude and Pi presets both decode JSONL files." — cierto para Claude y Pi, pero OpenCode usa un reader SQLite dedicado (`decode.format = "opencode"`, lee `~/.local/share/opencode/opencode.db`). Sin esta aclaración, un usuario que intente adaptar el preset de OpenCode puede no entender por qué no tiene campos `[inputs.record]`, `[inputs.map]`, etc.

## Alcance

**In**:
1. Línea 113: expandir "Claude and Pi presets" → "Claude, Pi, and OpenCode presets" (con qualifier: OpenCode ships inactive by default)
2. Línea 117: agregar frase sobre el reader SQLite de OpenCode inmediatamente después de la frase sobre JSONL

**Out**:
- No restructurar la sección "Session File Format"
- No agregar subsecciones nuevas
- No modificar otros archivos

## Estado inicial esperado

- `grep -c "Claude and Pi presets" docs/sync.md` retorna ≥ 2

## Criterios de Aceptación

- `grep "OpenCode" docs/sync.md` encuentra al menos 1 match en la sección "Session File Format"
- `grep "opencode\|SQLite\|sqlite" docs/sync.md` encuentra match relacionado a formato
- `just check` pasa

## Fuente de verdad

- `docs/sync.md` — archivo a modificar
- `inputs/opencode.inputs.toml` — confirma `decode.format = "opencode"` y root `~/.local/share/opencode`
