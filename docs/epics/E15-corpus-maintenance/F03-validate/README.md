# F03: Validate

**Epic**: [E15 Corpus Maintenance](../README.md)
**Objetivo**: Detectar problemas de integridad entre archivos fuente y el indice SQLite sin modificar datos
**Satisface**: P3 (validate reporta integridad)
**Milestone**: `backscroll validate` ejecuta queries de integridad y reporta discrepancias

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E15)
- INV2: `just check` pasa (heredado de E15)
- INV4: validate es read-only (heredado de E15)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S063](S063-integrity-validation/) | Integrity Validation |
