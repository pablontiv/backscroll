---
estado: Completed
tipo: chore
ejecutable_en: 1 sesion
---
# T021: Crear archivo LICENSE (MIT)

**Story**: [S028 Limpieza tecnica](README.md)
**Contribuye a**: LICENSE file existe en la raiz del repo

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El repositorio no tiene archivo LICENSE. Crear LICENSE con MIT license. Verificar que `license` en Cargo.toml dice "MIT".

## Alcance

**In**:
1. Crear archivo `LICENSE` en la raiz con texto MIT
2. Verificar que Cargo.toml tiene `license = "MIT"` (agregar si falta)

**Out**: No modificar README ni CLAUDE.md.

## Estado inicial esperado

- No existe archivo LICENSE en la raiz

## Criterios de Aceptacion

- `test -f LICENSE` retorna 0 (archivo existe)
- `grep "MIT" LICENSE` encuentra "MIT License"
- `grep 'license = "MIT"' Cargo.toml` encuentra la declaracion
- `just check` pasa

## Fuente de verdad

- `LICENSE` (raiz del repo)
- `Cargo.toml`
