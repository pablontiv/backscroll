# E06: Robustez del Motor

**Objetivo**: Corregir el parser para JSONL real de Claude Code, migrar a schema extensible con content table, implementar el patrón ports & adapters completo, y limpiar dead code.

## Postcondiciones

| # | Postcondicion | Features | Verificacion |
|---|---------------|----------|-------------|
| P1 | Parser procesa JSONL real de Claude Code (wrapper format) | F01 | `cargo test test_parse_real_jsonl` con fixture real pasa |
| P2 | Schema tiene content table con source, timestamp, ordinal, uuid | F01 | `sqlite3 test.db ".schema search_items"` muestra todas las columnas |
| P3 | SearchEngine trait implementado, main.rs usa dyn SearchEngine | F02 | `grep -r "dyn SearchEngine" src/main.rs` encuentra uso |
| P4 | Re-sync no produce duplicados | F01 | `cargo test test_resync_no_duplicates` pasa |
| P5 | Tracing instrumentado, zero dead code | F03 | `just check` pasa, `grep -r "allow(dead_code)" src/` = 0 |

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa (fmt + clippy -D warnings)

## Out of Scope

- Filtrado de ruido por contenido (E07)
- Output enriquecido (E08)
- Cross-platform builds (Windows/macOS)

## Features

| Feature | Descripcion |
|---------|-------------|
| [F01](F01-parser-schema/) | Parser y Schema |
| [F02](F02-ports-adapters/) | Ports & Adapters |
| [F03](F03-instrumentacion-limpieza/) | Instrumentacion y Limpieza |

## Dependencias

E06 es prerrequisito de E07 y E08.
