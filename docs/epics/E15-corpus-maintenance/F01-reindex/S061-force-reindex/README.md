# S061: Force Reindex

**Feature**: [F01 Reindex](../README.md)
**Capacidad**: Un subcomando reconstruye el indice completo desde archivos fuente
**Cubre**: P1 (reindex reconstruye indice)

## Antes / Despues

**Antes**: Si los filtros de ruido cambian (core/sync.rs regexes), los mensajes ya indexados no se re-procesan porque sus hashes no cambian. No hay forma de forzar re-indexacion sin borrar la DB manualmente (`rm ~/.backscroll.db`).

**Despues**: `backscroll reindex` borra todos los hashes en indexed_files (forzando que parse_sessions re-procese todos los archivos), ejecuta sync completo, y reconstruye el indice FTS5 via triggers. La DB se mantiene intacta — solo se re-procesan los datos.

## Criterios de Aceptacion (semanticos)

- [ ] `backscroll reindex` re-procesa todos los archivos independientemente del hash
- [ ] FTS5 index es consistente con los datos re-procesados
- [ ] Operacion es atomica (transaccion SQL)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T101](T101-reindex-subcommand.md) | Add reindex subcommand with clear hashes + force re-sync |
| [T102](T102-reindex-test.md) | Integration test for reindex |

## Fuente de verdad

- `src/main.rs` — CLI commands
- `src/storage/sqlite.rs` — indexed_files table, sync_files()
- `src/core/sync.rs` — parse_sessions()
