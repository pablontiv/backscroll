# F02: Binary Distribution

**Epic**: [E13 Release Pipeline](../README.md)
**Objetivo**: Binarios pre-compilados disponibles en GitHub Releases para Linux y macOS
**Satisface**: P1 (binarios en Release), P3 (instalacion sin Rust)
**Milestone**: GitHub Release de un tag produce automaticamente binarios Linux musl y macOS aarch64 descargables

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E13)
- INV2: `just check` pasa (heredado de E13)
- INV3: CI existente no se rompe (heredado de E13)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S055](S055-cross-platform-ci-builds/) | Cross-platform CI Builds |
| [S056](S056-install-script/) | Install Script |

## Dependencias

- F01 (Release Artifacts) — version y metadata deben estar listos
