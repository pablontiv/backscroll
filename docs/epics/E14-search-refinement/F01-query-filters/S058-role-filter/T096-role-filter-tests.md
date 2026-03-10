---
ejecutable_en: 1 sesion
estado: Completed # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T096: Integration tests for role filtering

**Story**: [S058 Role Filter](README.md)
**Contribuye a**: Tests verifican que role filtering funciona end-to-end

[[blocks:T095-role-flag-where-clause]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Siguiendo el patron de T094 (date filter tests), crear tests de integracion para role filtering usando fixtures con mensajes de ambos roles.

## Alcance

**In**:
1. Fixture con mensajes de role "user" y "assistant"
2. Test: --role human retorna solo mensajes user
3. Test: --role assistant retorna solo mensajes assistant
4. Test: sin --role retorna ambos

**Out**: Date filter tests (T094)

## Estado inicial esperado

- T095 completado (--role flag funcional)
- tests/cli.rs con patron de fixtures

## Criterios de Aceptacion

- Al menos 3 tests nuevos para role filtering
- `cargo test test_search_role` pasa

## Fuente de verdad

- `tests/cli.rs`
