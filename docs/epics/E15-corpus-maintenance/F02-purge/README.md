# F02: Purge

**Epic**: [E15 Corpus Maintenance](../README.md)
**Objetivo**: Eliminar datos antiguos del indice para reducir tamano de DB y mejorar performance de queries
**Satisface**: P2 (purge por fecha)
**Milestone**: `backscroll purge --before 2025-01-01` elimina entries, limpia orphans, y ejecuta VACUUM

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E15)
- INV2: `just check` pasa (heredado de E15)
- INV3: Purge es transaccional (heredado de E15)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S062](S062-purge-old-data/) | Purge Old Data |
