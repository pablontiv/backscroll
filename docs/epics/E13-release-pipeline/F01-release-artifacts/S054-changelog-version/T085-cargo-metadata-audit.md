---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T085: Audit Cargo.toml metadata for v1.0

**Story**: [S054 Changelog & Version](README.md)
**Contribuye a**: Cargo.toml tiene todos los campos requeridos para publicacion

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Cargo.toml en v0.1.14 tiene metadata basica. Para v1.0, se necesita metadata completa: description, repository, homepage, documentation, categories, keywords, readme. Esto es necesario para crates.io y para que GitHub muestre metadata correcta.

## Alcance

**In**:
1. Audit campos existentes en Cargo.toml
2. Agregar/actualizar: description, repository, homepage, documentation, categories, keywords, readme
3. Verificar que `cargo package --list` no incluye archivos innecesarios
4. Agregar `.cargo/config.toml` si se necesita para package exclude

**Out**: Bump de version (T086)

## Estado inicial esperado

- Cargo.toml con version 0.1.14, name, edition, license

## Criterios de Aceptacion

- `cargo metadata --format-version 1 | jq '.packages[0].description'` no es null
- `cargo metadata --format-version 1 | jq '.packages[0].repository'` apunta al repo correcto
- `cargo package --list` no incluye archivos de docs/research ni fixtures de test
- Todos los campos recomendados por crates.io estan presentes

## Fuente de verdad

- `Cargo.toml`
