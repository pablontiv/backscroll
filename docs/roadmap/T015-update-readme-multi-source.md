---
estado: Completed
tipo: task
---
# T015: Actualizar README para reflejar soporte de Claude, Pi y OpenCode

**Contribuye a**: README sin referencias desactualizadas a "Claude/Pi" que omiten OpenCode

## Preserva

- INV1: `just check` pasa
  - Verificar: `just check`

## Contexto

README.md menciona "Claude/Pi" en 6 lugares y omite OpenCode completamente. Además, los comandos de instalación manual (`cp` en bash y `foreach` en PowerShell) copian solo 2 presets cuando en realidad se envían 4. OpenCode usa un reader SQLite (`decode.format = "opencode"`) mientras Claude y Pi usan JSONL — esa distinción vale una frase porque no es obvia para quien quiera escribir un manifest custom.

Ocurrencias a corregir:
- **Líneas 38 y 46**: `"shipped Claude/Pi input presets"` → `"shipped Claude, Pi, and OpenCode input presets"`
- **Línea 50**: párrafo que lista solo `claude.inputs.toml` + `pi.inputs.toml` — actualizar para incluir `opencode.inputs.toml` y aclarar que OpenCode viene con `active = false` (opt-in)
- **Línea 67** (bash cp): `inputs/claude.inputs.toml inputs/pi.inputs.toml` → agregar `inputs/opencode.inputs.toml`
- **Línea 76** (PS foreach): `"claude.inputs.toml", "pi.inputs.toml"` → agregar `"opencode.inputs.toml"`
- **Línea 139**: `"Claude Code and Pi produce valuable reasoning logs"` → `"Claude Code, Pi, and OpenCode produce valuable reasoning logs"`
- **Línea 152**: `"the shipped Claude and Pi presets handle their respective JSONL formats"` → agregar: OpenCode preset reads from OpenCode's SQLite database (`decode.format = "opencode"`) rather than JSONL

## Alcance

**In**:
1. Actualizar todas las ocurrencias de "Claude/Pi" en las líneas descritas
2. Agregar una frase sobre SQLite en la sección "The Session Index" (línea ~152)
3. Incluir `opencode.inputs.toml` en los comandos de instalación manual

**Out**:
- No restructurar secciones
- No agregar tablas nuevas ni secciones "Supported Sources"
- No modificar otros archivos

## Estado inicial esperado

- `grep -c "Claude/Pi\|Claude and Pi" README.md` retorna ≥ 4

## Criterios de Aceptación

- `grep -c "Claude/Pi\|Claude and Pi" README.md` retorna 0
- `grep "opencode.inputs.toml" README.md` encuentra al menos 2 matches (cp bash + descripción)
- `grep "SQLite\|opencode.*sqlite\|opencode.*db" README.md` encuentra al menos 1 match
- `just check` pasa

## Fuente de verdad

- `README.md` — archivo a modificar
- `inputs/opencode.inputs.toml` — confirma `active = false` y `decode.format = "opencode"`
