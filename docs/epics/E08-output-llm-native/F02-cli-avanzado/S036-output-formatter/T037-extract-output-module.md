---
estado: Completed
tipo: refactor
ejecutable_en: 1 sesion
---
# T037: Extraer output formatting a src/output.rs

**Story**: [S036 Output formatter + flags estructurados](README.md)
**Contribuye a**: Output formatting modular

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Contexto

El formatting de output esta inline en main.rs despues de S035. Extraer a `src/output.rs` como modulo dedicado para que los distintos formatos (text, json, robot) se implementen limpiamente.

## Alcance

**In**:
1. Crear `src/output.rs` con trait o enum para formatos de output
2. Mover logica de formateo de main.rs a output.rs
3. main.rs llama `output::format_results(results, format)` o similar
4. Registrar modulo en main.rs (`mod output;`)

**Out**: No implementar json/robot (T038). Solo extraer texto plano.

## Estado inicial esperado

- Formatting inline en main.rs (S035 completado)

## Criterios de Aceptacion

- `test -f src/output.rs` — modulo existe
- main.rs no tiene logica de formateo inline
- `cargo test` pasa (snapshot test sigue pasando)
- `just check` pasa

## Fuente de verdad

- `src/output.rs` (nuevo)
- `src/main.rs`
