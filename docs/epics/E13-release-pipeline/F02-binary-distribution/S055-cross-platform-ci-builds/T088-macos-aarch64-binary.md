---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T088: Add macOS aarch64 binary to CI release job

**Story**: [S055 Cross-platform CI Builds](README.md)
**Contribuye a**: GitHub Release contiene backscroll-macos-aarch64

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

GitHub Actions puede compilar para macOS usando runners `macos-latest` (aarch64). Se necesita agregar un step o job paralelo que compile para `aarch64-apple-darwin` y suba el binario. SQLite bundled (rusqlite feature) elimina dependencias de sistema, pero se necesita verificar que la compilacion cross funciona.

## Alcance

**In**:
1. Agregar job o step para macOS aarch64 en CI release workflow
2. Compilar con `cargo build --release --target aarch64-apple-darwin`
3. Renombrar binario a `backscroll-macos-aarch64`
4. Upload como asset del GitHub Release

**Out**: Linux target (T087), smoke test (T089)

## Estado inicial esperado

- ci.yml con release job funcional
- rusqlite con feature `bundled` (no system SQLite dependency)

## Criterios de Aceptacion

- CI produce artifact `backscroll-macos-aarch64`
- Binario es un Mach-O aarch64 ejecutable
- Binario aparece en GitHub Release assets

## Fuente de verdad

- `.github/workflows/ci.yml` — release job
