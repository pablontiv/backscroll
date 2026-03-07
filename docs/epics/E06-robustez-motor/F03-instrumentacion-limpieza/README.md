# F03: Instrumentacion y Limpieza

**Epic**: [E06 Robustez del Motor](../README.md)
**Objetivo**: Activar tracing real (completa S020 que tiene crates pero zero uso), eliminar dead code y legacy artifacts.
**Satisface**: P5 (tracing instrumentado, zero dead code)
**Milestone**: `RUST_LOG=debug backscroll sync` muestra spans; `grep -r "allow(dead_code)" src/` = 0.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E06)
- INV2: `just check` pasa (heredado de E06)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S027](S027-tracing/) | Tracing (completa S020) |
| [S028](S028-limpieza-tecnica/) | Limpieza tecnica |
