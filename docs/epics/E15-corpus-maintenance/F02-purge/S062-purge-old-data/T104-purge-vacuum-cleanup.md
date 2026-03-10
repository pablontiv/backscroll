---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T104: Implement DELETE + VACUUM + orphan cleanup

**Story**: [S062 Purge Old Data](README.md)
**Contribuye a**: Purge elimina datos, limpia orphans, y recupera espacio

[[blocks:T103-purge-subcommand]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV3: Purge es transaccional
  - Verificar: DB no queda en estado inconsistente si falla

## Contexto

Despues de DELETE FROM search_items WHERE timestamp < ?, quedan entries huerfanas en indexed_files (paths que ya no tienen search_items). Hay que limpiarlas. Luego, VACUUM recupera espacio en disco. VACUUM no puede ejecutarse dentro de una transaccion en SQLite, asi que el flujo es: BEGIN → DELETE + orphan cleanup → COMMIT → VACUUM.

## Alcance

**In**:
1. Dentro de purge(): DELETE FROM search_items WHERE timestamp < ? AND source = 'session'
2. Orphan cleanup: DELETE FROM indexed_files WHERE path NOT IN (SELECT DISTINCT source_path FROM search_items)
3. Reportar conteos (PurgeStats)
4. Post-transaccion: PRAGMA vacuum
5. Imprimir resumen: "Purged N items from M files. DB size: X → Y"

**Out**: Tests (T105)

## Estado inicial esperado

- T103 completado (purge trait method existe)

## Criterios de Aceptacion

- search_items con timestamp < before son eliminados
- indexed_files huerfanos son eliminados
- VACUUM ejecutado post-transaccion
- Purge reporta conteos de items y files eliminados
- `cargo test --all-features` pasa

## Fuente de verdad

- `src/storage/sqlite.rs` — purge implementation
