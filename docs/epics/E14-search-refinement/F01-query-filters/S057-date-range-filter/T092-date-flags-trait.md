---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T092: Add --after/--before CLI flags and extend SearchEngine trait

**Story**: [S057 Date Range Filter](README.md)
**Contribuye a**: --after/--before flags existen en CLI y SearchEngine::search() acepta parametros de fecha

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El comando Search en main.rs (lineas 40-65) tiene flags para project, source, json, robot, fields, max_tokens. Se necesitan dos flags nuevos: `--after` y `--before` que acepten strings de fecha (ISO 8601 parcial, ej: "2026-03-01"). El SearchEngine trait (core/mod.rs:65-83) tiene `search(query, project, source)` — se necesita extender con parametros de fecha. La implementacion SQL se hace en T093.

## Alcance

**In**:
1. Agregar `--after` y `--before` como `Option<String>` al struct Search en main.rs
2. Extender `SearchEngine::search()` signature con parametros `after: &Option<String>`, `before: &Option<String>`
3. Actualizar la llamada en main.rs que invoca search()
4. Actualizar la implementacion stub en sqlite.rs (solo pasar parametros, sin WHERE aun)

**Out**: Implementacion SQL del WHERE clause (T093), tests (T094)

## Estado inicial esperado

- Search struct en main.rs con 7 flags existentes
- SearchEngine trait con search(query, project, source)

## Criterios de Aceptacion

- `backscroll search "test" --after 2026-03-01 --help` muestra el flag sin error
- `backscroll search "test" --before 2026-03-09 --help` muestra el flag sin error
- SearchEngine trait compila con nuevos parametros
- `cargo test --all-features` pasa (no hay test de funcionalidad aun)

## Fuente de verdad

- `src/main.rs:40-65` — Search command struct
- `src/core/mod.rs:65-83` — SearchEngine trait
- `src/storage/sqlite.rs` — impl SearchEngine for Database
