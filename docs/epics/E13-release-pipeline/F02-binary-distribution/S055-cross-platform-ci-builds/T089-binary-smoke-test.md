---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T089: Binary portability smoke test in CI

**Story**: [S055 Cross-platform CI Builds](README.md)
**Contribuye a**: Binarios descargados funcionan sin dependencias de runtime

[[blocks:T087-linux-musl-binary]]
[[blocks:T088-macos-aarch64-binary]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Los binarios pre-compilados deben funcionar sin dependencias. Un smoke test basico verifica que el binario se ejecuta correctamente en un entorno limpio (sin Rust toolchain). Para Linux musl esto se puede verificar en el mismo runner. Para macOS se usa el runner macos-latest.

## Alcance

**In**:
1. Agregar step post-build que ejecute `./backscroll-linux-x86_64 --version` en Linux runner
2. Agregar step post-build que ejecute `./backscroll-macos-aarch64 --version` en macOS runner
3. Verificar que `backscroll sync --help` funciona (ejerce clap parsing)

**Out**: Test funcional completo (sync/search) — solo smoke test

## Estado inicial esperado

- T087 y T088 producen binarios en CI

## Criterios de Aceptacion

- `./backscroll-linux-x86_64 --version` retorna exit 0 con version string
- `./backscroll-macos-aarch64 --version` retorna exit 0 con version string
- `./backscroll-linux-x86_64 sync --help` retorna exit 0

## Fuente de verdad

- `.github/workflows/ci.yml` — release job
