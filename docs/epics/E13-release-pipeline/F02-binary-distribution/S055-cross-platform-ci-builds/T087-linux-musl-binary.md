---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T087: Add Linux musl static binary to CI release job

**Story**: [S055 Cross-platform CI Builds](README.md)
**Contribuye a**: GitHub Release contiene backscroll-linux-x86_64 (musl static)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV3: CI gates existentes siguen pasando
  - Verificar: Push a branch, CI pasa

## Contexto

`just static-build` ya usa `cargo zigbuild --release --target x86_64-unknown-linux-musl` para generar binarios estaticos. El job de release en CI (ci.yml:59-148) compila con `cargo auditable build --release` pero solo para el target nativo. Se necesita agregar cross-compilation a musl en el job de release y subir el binario como asset.

## Alcance

**In**:
1. Agregar step en CI release job para instalar cargo-zigbuild
2. Compilar con `cargo zigbuild --release --target x86_64-unknown-linux-musl`
3. Renombrar binario a `backscroll-linux-x86_64`
4. Upload como asset del GitHub Release via `gh release upload`

**Out**: macOS target (T088), smoke test (T089)

## Estado inicial esperado

- ci.yml con job de release funcional
- cargo-zigbuild configurado en Cargo.toml/build profiles

## Criterios de Aceptacion

- CI release job produce artifact `backscroll-linux-x86_64`
- Binario es estaticamente linkeado: `file backscroll-linux-x86_64` muestra "statically linked"
- Binario aparece en GitHub Release assets

## Fuente de verdad

- `.github/workflows/ci.yml` — release job
- `Justfile:26-29` — static-build recipe (reference)
