# E13: Release Pipeline

**Metrica de exito**: Usuarios instalan backscroll sin Rust toolchain via binario pre-compilado o install script
**Timeline**: 2026-Q1 — en curso

## Intencion

Backscroll v0.1.14 tiene funcionalidad completa (E01-E12) pero solo es instalable via `cargo install`. Este epic establece la pipeline de release para v1.0: CHANGELOG automatico, binarios multi-plataforma en GitHub Releases, e install script.

## Postcondiciones

- P1: GitHub Release contiene binarios pre-compilados (Linux musl x86_64, macOS aarch64)
- P2: CHANGELOG.md existe y se genera automaticamente desde commits convencionales
- P3: Usuarios pueden instalar sin Rust toolchain via install script o descarga directa

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa
- INV3: CI existente no se rompe (gates check-lint, test, audit, gitleaks siguen pasando)

## Out of Scope

- Homebrew formula, cargo-binstall support
- Windows binaries
- Firma de binarios (code signing)

## Features

| ID | Nombre | Descripcion |
|----|--------|-------------|
| F01 | [Release Artifacts](F01-release-artifacts/) | CHANGELOG y metadata de version para v1.0 |
| F02 | [Binary Distribution](F02-binary-distribution/) | Binarios multi-plataforma en GitHub Releases + install script |

## Orden de Ejecucion

| Feature | Depende de | Razon |
|---------|-----------|-------|
| F01 | — | Artifacts primero (CHANGELOG, version) |
| F02 | F01 | Distribucion despues de que version y metadata estan listos |

## Decision Log

| Fecha | Decision | Razon |
|-------|----------|-------|

## Gaps Activos

- Ninguno identificado
