---
estado: Specified
tipo: task
---
# T014: Agregar inputs/pi.inputs.toml al repositorio

**Contribuye a**: inputs/pi.inputs.toml debe existir en el repo como asset de instalación

## Preserva

- INV1: `just check` pasa (gofmt + go vet)
  - Verificar: `just check`

## Contexto

`install.sh` y `install.ps1` declaran `pi.inputs.toml` en su array `INPUT_PRESETS` (línea 6 de install.sh), pero el archivo no existe en `inputs/`. La versión correcta vive en `~/.config/backscroll/inputs/pi.inputs.toml` del usuario. El archivo en `tests/fixtures/pi.inputs.toml` tiene roots incorrectos (`.` / `pi-session.jsonl`) y no debe usarse como fuente.

El preset correcto usa:
- `roots = ["~/.pi/agent/sessions", "~/.pi/agent/sessions-archive"]`
- `include = ["**/*.jsonl"]`
- `active = true`

## Alcance

**In**:
1. Crear `inputs/pi.inputs.toml` con el contenido de `~/.config/backscroll/inputs/pi.inputs.toml`
2. Agregar comentario de cabecera consistente con el de `inputs/opencode.inputs.toml`

**Out**:
- No modificar tests/fixtures/pi.inputs.toml
- No modificar install scripts

## Estado inicial esperado

- `inputs/pi.inputs.toml` NO existe en el repo
- `~/.config/backscroll/inputs/pi.inputs.toml` existe y tiene contenido correcto

## Criterios de Aceptación

- `ls inputs/pi.inputs.toml` retorna exit 0
- `grep "~/.pi/agent/sessions" inputs/pi.inputs.toml` encuentra match
- `just check` pasa

## Fuente de verdad

- `~/.config/backscroll/inputs/pi.inputs.toml` — contenido fuente
- `inputs/opencode.inputs.toml` — referencia para formato de comentario de cabecera
- `install.sh` línea 6 — confirma que pi.inputs.toml debe existir
