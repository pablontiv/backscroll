---
estado: Pending
tipo: refactor
ejecutable_en: 1 sesion
---
# T015: Refactorizar sync.rs a funcion pura

**Story**: [S026 Refactorizar sync y main](README.md)
**Contribuye a**: sync.rs no importa Database directamente

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

`sync.rs` actualmente importa `crate::storage::sqlite::Database` y ejecuta side effects directamente (INSERT). Debe refactorizarse a una funcion pura que retorne `Vec<ParsedFile>` sin conocer el storage backend. main.rs se encargara de pasar los files al engine.

## Alcance

**In**:
1. Cambiar `sync_sessions(db, path)` a `parse_sessions(path, existing_hashes) -> Vec<ParsedFile>`
2. Remover import de `crate::storage::sqlite::Database` de sync.rs
3. La funcion recibe hashes existentes (de `engine.get_file_hashes()`) para skip unchanged
4. Retorna Vec<ParsedFile> sin ejecutar side effects

**Out**: No modificar main.rs (T016). Mantener tests funcionando.

## Estado inicial esperado

- impl SearchEngine for Database completado (S025)
- sync.rs importa Database y ejecuta INSERTs directamente

## Criterios de Aceptacion

- `grep "Database" src/core/sync.rs` no encuentra imports de sqlite::Database
- sync.rs exporta una funcion que retorna `Vec<ParsedFile>`
- `cargo test` pasa
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs`
