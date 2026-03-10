---
ejecutable_en: 1 sesion
estado: Completed # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T094: Integration tests for date range filtering

**Story**: [S057 Date Range Filter](README.md)
**Contribuye a**: Tests verifican que date filtering funciona end-to-end

[[blocks:T093-timestamp-where-clause]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Los tests de integracion en tests/cli.rs usan `assert_cmd` + `predicates` y crean fixtures JSONL con timestamps. Se necesitan tests que:
1. Creen sessions con timestamps conocidos
2. Busquen con --after y --before
3. Verifiquen que solo los resultados dentro del rango aparecen

## Alcance

**In**:
1. Crear fixture JSONL con 3+ mensajes con timestamps distintos (ej: "100", "200", "300")
2. Test: --after filtra correctamente
3. Test: --before filtra correctamente
4. Test: --after + --before combinados
5. Test: sin flags temporales retorna todos (backward compat)

**Out**: Tests de role filter (T096)

## Estado inicial esperado

- T092 y T093 completados
- tests/cli.rs con patron de fixtures existente (sync_fixture function)

## Criterios de Aceptacion

- Al menos 4 tests nuevos en tests/cli.rs para date filtering
- Tests cubren: after-only, before-only, both, neither
- `cargo test test_search_date` pasa

## Fuente de verdad

- `tests/cli.rs` — integration tests
