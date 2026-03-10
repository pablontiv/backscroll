# F01: Reindex

**Epic**: [E15 Corpus Maintenance](../README.md)
**Objetivo**: Reconstruir el indice FTS5 desde archivos fuente cuando los filtros de ruido cambian o el indice se corrompe
**Satisface**: P1 (reindex reconstruye indice)
**Milestone**: `backscroll reindex` borra hashes en indexed_files, fuerza re-parse de todos los archivos, y reconstruye FTS5

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E15)
- INV2: `just check` pasa (heredado de E15)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S061](S061-force-reindex/) | Force Reindex |
