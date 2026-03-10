---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: test # [code, test, refactor, chore, docs]
---
# T105: Integration test for purge

**Story**: [S062 Purge Old Data](README.md)
**Contribuye a**: Test verifica purge end-to-end

[[blocks:T104-purge-vacuum-cleanup]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Test que: 1) sync fixtures con timestamps variados, 2) purge --before timestamp medio, 3) verify que solo quedan los recientes, 4) verify status count reduced.

## Alcance

**In**:
1. Fixture con mensajes timestamp "100", "200", "300"
2. Purge --before "250"
3. Verify search retorna solo mensajes con timestamp >= "250"
4. Verify status muestra count reducido

**Out**: Tests de otros subcommands

## Estado inicial esperado

- T104 completado
- tests/cli.rs con sync_fixture

## Criterios de Aceptacion

- Test `test_purge` pasa
- Verifica que purge elimina entries antiguas
- Verifica que entries recientes sobreviven

## Fuente de verdad

- `tests/cli.rs`
