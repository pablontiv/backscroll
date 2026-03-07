---
estado: Pending
tipo: refactor
ejecutable_en: 1 sesion
---
# T016: Refactorizar main.rs con factory dyn SearchEngine

**Story**: [S026 Refactorizar sync y main](README.md)
**Contribuye a**: main.rs usa dyn SearchEngine

[[blocks:T015-sync-pure-function]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

main.rs debe usar el trait `SearchEngine` en lugar de `Database` directamente, habilitando futuros backends alternativos. Un factory `create_engine()` retorna `Box<dyn SearchEngine>`.

## Alcance

**In**:
1. Crear funcion `create_engine(config) -> Box<dyn SearchEngine>` en main.rs
2. Cambiar sync command: `let files = parse_sessions(path, engine.get_file_hashes()?); engine.sync_files(files)?`
3. Cambiar search command: `engine.search(query, project)?`
4. Database solo se construye dentro de create_engine

**Out**: No agregar nuevos backends.

## Estado inicial esperado

- sync.rs retorna Vec<ParsedFile> (T015)
- impl SearchEngine for Database (T013)

## Criterios de Aceptacion

- `grep "dyn SearchEngine" src/main.rs` encuentra uso
- `grep "create_engine" src/main.rs` encuentra la factory
- `cargo test` pasa
- `just check` pasa

## Fuente de verdad

- `src/main.rs`
