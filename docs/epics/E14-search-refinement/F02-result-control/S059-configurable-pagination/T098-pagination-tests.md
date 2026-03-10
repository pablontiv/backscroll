---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T098: Integration tests for pagination

**Story**: [S059 Configurable Pagination](README.md)
**Contribuye a**: Tests verifican pagination end-to-end

[[blocks:T097-limit-offset-flags]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Tests que verifican --limit y --offset usando fixtures con multiples mensajes.

## Alcance

**In**:
1. Fixture con 25+ mensajes indexados
2. Test: --limit 5 retorna exactamente 5
3. Test: --limit 0 retorna todos
4. Test: --offset 10 salta primeros 10
5. Test: default (sin flags) retorna max 20

**Out**: Tests de otros filters

## Estado inicial esperado

- T097 completado
- tests/cli.rs

## Criterios de Aceptacion

- Al menos 4 tests para pagination
- `cargo test test_search_pagination` pasa

## Fuente de verdad

- `tests/cli.rs`
