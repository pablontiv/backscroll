# S062: Purge Old Data

**Feature**: [F02 Purge](../README.md)
**Capacidad**: Un subcomando elimina entries anteriores a una fecha del indice
**Cubre**: P2 (purge por fecha)

## Antes / Despues

**Antes**: No hay forma de reducir el tamano de la DB. Todas las sesiones indexadas permanecen indefinidamente. Para corpus de 1000+ sesiones, la DB puede crecer significativamente.

**Despues**: `backscroll purge --before 2025-01-01` elimina search_items con timestamp < fecha, limpia entries huerfanas en indexed_files, y ejecuta VACUUM para recuperar espacio. La operacion es transaccional.

## Criterios de Aceptacion (semanticos)

- [ ] `backscroll purge --before DATE` elimina entries anteriores
- [ ] Entries huerfanas en indexed_files se limpian
- [ ] VACUUM reduce tamano de DB
- [ ] Operacion es transaccional (rollback si falla)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Purge es transaccional
  - Verificar: Test que verifica rollback en caso de error

## Tasks

| Task | Descripcion |
|------|-------------|
| [T103](T103-purge-subcommand.md) | Add purge subcommand with --before date filter |
| [T104](T104-purge-vacuum-cleanup.md) | Implement DELETE + VACUUM + orphan cleanup |
| [T105](T105-purge-test.md) | Integration test for purge |

## Fuente de verdad

- `src/main.rs` — CLI commands
- `src/storage/sqlite.rs` — search_items, indexed_files tables
