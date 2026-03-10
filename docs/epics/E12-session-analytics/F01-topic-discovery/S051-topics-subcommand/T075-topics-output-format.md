---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T075: Output formatting for topics

**Story**: [S051 Topics subcommand & output](README.md)
**Contribuye a**: P1 (topics retorna terminos rankeados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Format topics output in text/robot/json consistent with existing search output.

## Especificacion Tecnica

Add topics formatting to output.rs:

- **Text**: `term (N sessions, M mentions)` per line
- **Robot**: tab-separated `term\tsessions\tmentions`
- **JSON**: `{"term":"x","sessions":N,"mentions":M}` per line

## Alcance

**In**: Add topics formatting to output.rs. Text, robot, and JSON formats.
**Out**: No CLI wiring (T074), no tests (T076)

## Criterios de Aceptacion

- Text output is human-readable
- Robot output is tab-separated without ANSI codes
- JSON output is valid JSON lines
- `just check` pasa

## Fuente de verdad

- `src/output.rs`
