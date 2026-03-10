# S063: Integrity Validation

**Feature**: [F03 Validate](../README.md)
**Capacidad**: Un subcomando reporta problemas de integridad del indice sin modificar datos
**Cubre**: P3 (validate reporta integridad)

## Antes / Despues

**Antes**: No hay forma de saber si el indice esta corrupto, si hay entries huerfanas (archivos borrados pero aun en DB), o si FTS5 esta desincronizado con search_items. El unico diagnostico es `backscroll status` que muestra conteos.

**Despues**: `backscroll validate` ejecuta queries de integridad: orphans en search_items (source files que ya no existen), stale entries en indexed_files, consistencia FTS5. Reporta discrepancias en formato legible. No modifica datos.

## Criterios de Aceptacion (semanticos)

- [ ] validate detecta orphaned search_items (source file no existe en disco)
- [ ] validate detecta stale indexed_files (no hay search_items correspondientes)
- [ ] validate reporta conteo de problemas encontrados
- [ ] validate es read-only (no modifica la DB)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV4: validate es read-only
  - Verificar: Comparar DB checksum antes/despues de validate

## Tasks

| Task | Descripcion |
|------|-------------|
| [T106](T106-validate-subcommand.md) | Add validate subcommand with integrity queries |
| [T107](T107-validate-test.md) | Integration test for validate |

## Fuente de verdad

- `src/main.rs` — CLI commands
- `src/storage/sqlite.rs` — search_items, indexed_files, messages_fts
