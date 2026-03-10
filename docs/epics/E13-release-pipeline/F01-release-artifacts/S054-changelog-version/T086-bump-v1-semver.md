---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T086: Bump to v1.0.0 and update post-1.0 semver in CI

**Story**: [S054 Changelog & Version](README.md)
**Contribuye a**: Version es v1.0.0 y CI aplica reglas semver post-1.0

[[blocks:T084-git-cliff-changelog]]
[[blocks:T085-cargo-metadata-audit]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: CI existente no se rompe
  - Verificar: Push a branch, CI pasa

## Contexto

CI tiene logica de auto-release con reglas pre-1.0 (feat→patch, feat!→minor). Despues de v1.0, debe cambiar a semver estandar (feat→minor, feat!→major). El bump a v1.0.0 es el trigger para activar las reglas post-1.0. La logica ya existe en ci.yml con un condicional `version >= 1.0`.

## Alcance

**In**:
1. Verificar que la logica post-1.0 en ci.yml funciona correctamente
2. Actualizar `just release-minor` y `just release-patch` si es necesario
3. Bump version a 1.0.0 via `cargo set-version 1.0.0`
4. Integrar git-cliff en el job de release para auto-update CHANGELOG

**Out**: Ejecutar el release real (eso lo hace CI en merge a master)

## Estado inicial esperado

- ci.yml con logica de auto-release (lineas 59-148)
- Justfile con recetas release-patch y release-minor
- T084 y T085 completados

## Criterios de Aceptacion

- `cargo metadata --format-version 1 | jq -r '.packages[0].version'` retorna "1.0.0"
- ci.yml tiene logica post-1.0 verificada (feat→minor, fix→patch, feat!→major)
- CHANGELOG se actualiza automaticamente en el job de release
- `just release-patch` funciona con las nuevas reglas

## Fuente de verdad

- `Cargo.toml` — version field
- `.github/workflows/ci.yml` — auto-release logic
- `Justfile` — release recipes
