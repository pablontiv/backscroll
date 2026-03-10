---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: test # [code, test, refactor, chore, docs]
---
# T102: Integration test for reindex

**Story**: [S061 Force Reindex](README.md)
**Contribuye a**: Test verifica que reindex reconstruye el indice

[[blocks:T101-reindex-subcommand]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Test que: 1) sync initial, 2) modify noise filter expectation, 3) reindex, 4) verify results change.
Para simplificar, el test puede: sync, verify count, reindex, verify count is same (confirms re-processing).

## Alcance

**In**:
1. Crear fixture JSONL y sync
2. Ejecutar `backscroll reindex`
3. Verificar que status muestra mismos conteos
4. Verificar exit code 0

**Out**: Tests de otros subcommands

## Estado inicial esperado

- T101 completado
- tests/cli.rs con sync_fixture helper

## Criterios de Aceptacion

- Test `test_reindex` pasa
- Test verifica que reindex completa sin error
- `cargo test test_reindex` exit 0

## Fuente de verdad

- `tests/cli.rs`
