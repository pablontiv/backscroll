# F02: CLI Avanzado

**Epic**: [E08 Output LLM-Native](../README.md)
**Objetivo**: Output formatter modular con flags --json, --robot, --fields, --max-tokens, y modo --read para lectura filtrada de sesiones.
**Satisface**: P2 (--json parseable)
**Milestone**: `backscroll search "test" --json | jq .` no falla.
**Fase**: E08-alpha (puede ejecutarse en paralelo con E07, solo necesita E06).

## Invariantes

- INV1: Busqueda sin flags produce output legible (heredado de E08)
- INV2: Performance < 1s en corpus de test (heredado de E08)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S036](S036-output-formatter/) | Output formatter + flags estructurados |
| [S037](S037-modo-read/) | Modo --read |
