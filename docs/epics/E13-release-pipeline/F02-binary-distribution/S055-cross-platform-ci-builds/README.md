# S055: Cross-platform CI Builds

**Feature**: [F02 Binary Distribution](../README.md)
**Capacidad**: GitHub Actions produce binarios multi-plataforma en cada release tag
**Cubre**: P1 (binarios en Release)

## Antes / Despues

**Antes**: Solo se compila con `cargo build --release` localmente. `just static-build` genera musl binary local pero no esta integrado en CI. GitHub Releases no contienen binarios.

**Despues**: El job de release en CI compila para Linux musl (x86_64) y macOS (aarch64), sube los binarios como assets del GitHub Release. Usuarios descargan el binario para su plataforma directamente.

## Criterios de Aceptacion (semanticos)

- [ ] GitHub Release contiene backscroll-linux-x86_64 (musl static)
- [ ] GitHub Release contiene backscroll-macos-aarch64
- [ ] Binarios descargados funcionan sin dependencias de runtime

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: CI gates existentes siguen pasando
  - Verificar: Push a branch, verificar CI

## Tasks

| Task | Descripcion |
|------|-------------|
| [T087](T087-linux-musl-binary.md) | Add Linux musl static binary target to CI release job |
| [T088](T088-macos-aarch64-binary.md) | Add macOS aarch64 binary target to CI release job |
| [T089](T089-binary-smoke-test.md) | Binary portability smoke test in CI |

## Fuente de verdad

- `.github/workflows/ci.yml` — CI workflow
- `Justfile` — static-build recipe (reference)
