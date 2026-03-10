# F01: Release Artifacts

**Epic**: [E13 Release Pipeline](../README.md)
**Objetivo**: CHANGELOG automatico y metadata de Cargo.toml lista para v1.0
**Satisface**: P2 (CHANGELOG), P3 (metadata correcta)
**Milestone**: `CHANGELOG.md` generado por git-cliff y `Cargo.toml` con metadata completa para crates.io/GitHub

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E13)
- INV2: `just check` pasa (heredado de E13)
- INV3: CI existente no se rompe (heredado de E13)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S054](S054-changelog-version/) | Changelog & Version |
