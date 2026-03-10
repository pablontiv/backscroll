# S054: Changelog & Version

**Feature**: [F01 Release Artifacts](../README.md)
**Capacidad**: CHANGELOG.md se genera automaticamente y metadata de Cargo.toml esta lista para v1.0
**Cubre**: CHANGELOG automatico + version metadata

## Antes / Despues

**Antes**: No existe CHANGELOG.md. Cargo.toml tiene metadata minima (v0.1.14). No hay registro de cambios entre releases. CI usa reglas pre-1.0 para semver bumps.

**Despues**: git-cliff genera CHANGELOG.md desde commits convencionales. Cargo.toml tiene metadata completa (description, repository, categories, keywords). Version bumped a v1.0.0 con reglas semver post-1.0 en CI.

## Criterios de Aceptacion (semanticos)

- [ ] CHANGELOG.md generado automaticamente con categorias por tipo de commit
- [ ] Cargo.toml tiene todos los campos requeridos para publicacion
- [ ] Version es v1.0.0 y CI aplica reglas semver post-1.0

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T084](T084-git-cliff-changelog.md) | Configure git-cliff and generate initial CHANGELOG.md |
| [T085](T085-cargo-metadata-audit.md) | Audit Cargo.toml metadata for v1.0 |
| [T086](T086-bump-v1-semver.md) | Bump to v1.0.0 and update post-1.0 semver logic in CI |

## Fuente de verdad

- `Cargo.toml` — version and metadata
- `.github/workflows/ci.yml` — auto-release logic with semver bump
- `Justfile` — release recipes
