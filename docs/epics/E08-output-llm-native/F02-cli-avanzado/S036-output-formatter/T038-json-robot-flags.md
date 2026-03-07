---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T038: Implementar --json y --robot

**Story**: [S036 Output formatter + flags estructurados](README.md)
**Contribuye a**: backscroll search "test" --json | jq . no falla

[[blocks:T037-extract-output-module]]

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Contexto

Dos modos de output adicionales: `--json` produce JSON lines (una linea JSON por resultado, parseable por jq), `--robot` produce formato compacto pipe-friendly (tab-separated, sin decoracion).

## Especificacion Tecnica

--json output (JSON lines):
```json
{"source_path":"session.jsonl","snippet":"...matched...","score":0.85,"timestamp":"2026-03-01"}
```

--robot output (TSV compacto):
```
session.jsonl\t0.85\t...matched...
```

## Alcance

**In**:
1. Agregar flags `--json` y `--robot` al comando Search (mutuamente excluyentes)
2. Implementar JsonFormatter y RobotFormatter en output.rs
3. JSON: serde_json::to_string para cada SearchResult
4. Robot: tab-separated minimal fields

**Out**: No implementar --fields/--max-tokens (T039).

## Estado inicial esperado

- output.rs existe como modulo (T037)
- Solo formato texto plano implementado

## Criterios de Aceptacion

- `backscroll search "test" --json | jq .` no falla
- `backscroll search "test" --robot | cut -f1` extrae primer campo
- `backscroll search "test"` (sin flags) sigue igual
- `just check` pasa

## Fuente de verdad

- `src/output.rs`
- `src/main.rs`
