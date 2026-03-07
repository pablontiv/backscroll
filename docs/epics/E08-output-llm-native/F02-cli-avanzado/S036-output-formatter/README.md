# S036: Output formatter + flags estructurados

**Feature**: [F02 CLI Avanzado](../README.md)
**Capacidad**: Output formatting extraido a modulo dedicado. Flags --json, --robot, --fields, --max-tokens disponibles.
**Cubre**: P2 del Epic (--json parseable)

## Antes / Despues

**Antes**: Output formatting inline en main.rs. Un solo formato de salida (texto plano). No hay modo JSON ni robot-friendly.

**Despues**: `src/output.rs` modulo dedicado. `--json` produce JSON lines. `--robot` produce formato compacto pipe-friendly. `--fields minimal|full` controla que campos se muestran. `--max-tokens N` trunca output a budget de tokens.

## Criterios de Aceptacion (semanticos)

- [ ] `backscroll search "test" --json | jq .` no falla
- [ ] `--robot` produce output sin decoracion
- [ ] `--max-tokens 500` trunca el output

## Invariantes

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado (default mode no cambia)
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Tasks

| Task | Descripcion |
|------|-------------|
| [T037](T037-extract-output-module.md) | Extraer output formatting a src/output.rs |
| [T038](T038-json-robot-flags.md) | Implementar --json y --robot |
| [T039](T039-fields-max-tokens.md) | Implementar --fields y --max-tokens |

## Fuente de verdad

- `src/output.rs` — modulo de output (nuevo)
- `src/main.rs` — CLI flags
