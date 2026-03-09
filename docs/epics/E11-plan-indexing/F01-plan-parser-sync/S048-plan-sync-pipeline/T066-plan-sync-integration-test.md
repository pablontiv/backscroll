---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T066: Plan sync integration test

**Story**: [S048 Plan sync pipeline](README.md)
**Contribuye a**: P1 (plans indexados), P2 (plans spliteados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Integration test end-to-end para plan sync.

## Especificacion Tecnica

En `tests/cli.rs` o `src/storage/sqlite.rs` tests:

1. Test: crear tempdir con plan .md files, sync, verificar `search_items` contiene entries con source='plan'
2. Test: verificar que secciones ## producen multiples rows
3. Test: verificar que sync incremental omite plans sin cambios (hash match)
4. Test: verificar que session entries no se ven afectadas

## Alcance

**In**: Integration tests para plan sync
**Out**: No test de --source filter (T069)

## Criterios de Aceptacion

- 4 tests
- `just test` pasa

## Fuente de verdad

- `tests/cli.rs`
- `src/storage/sqlite.rs` — mod tests
