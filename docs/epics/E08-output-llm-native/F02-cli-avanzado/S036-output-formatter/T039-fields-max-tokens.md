---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T039: Implementar --fields y --max-tokens

**Story**: [S036 Output formatter + flags estructurados](README.md)
**Contribuye a**: --max-tokens 500 trunca el output

[[blocks:T038-json-robot-flags]]

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Contexto

Para uso con LLMs, se necesita controlar el volumen de output. `--fields minimal|full` controla que campos se incluyen. `--max-tokens N` trunca el output total a un budget aproximado de tokens (estimado como chars/4).

## Alcance

**In**:
1. Flag `--fields minimal|full` (default: minimal = source_path + snippet + score; full = todos los campos)
2. Flag `--max-tokens N` (default: sin limite)
3. Estimacion de tokens: chars / 4 (heuristica simple)
4. Truncar resultados cuando el budget se agota

**Out**: No implementar tokenizer preciso.

## Estado inicial esperado

- --json y --robot implementados (T038)
- output.rs maneja multiples formatos

## Criterios de Aceptacion

- `backscroll search "test" --max-tokens 100` produce output truncado
- `backscroll search "test" --fields full --json` incluye todos los campos
- `just check` pasa

## Fuente de verdad

- `src/output.rs`
- `src/main.rs`
