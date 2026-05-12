---
estado: Completed
tipo: task
---
# T004: Actualizar callers de setup_schema en tests a &mut self

**Outcome**: [Migrar setup_schema a refinery](README.md)

## Preserva

- La lógica de cada test no cambia — solo la mutabilidad de la variable db.
- Todos los tests existentes siguen pasando.

## Contexto

setup_schema() cambia a &mut self. Rust requiere que la variable binding sea mut para llamar métodos &mut self. Los tests usan let db = Database::open(...) — si ya tienen mut es no-op, si no lo tienen hay que agregarlo.

## Alcance

**In**:
1. Agregar mut a las declaraciones let db en tests que llaman setup_schema() y no tienen mut.

**Out**:
1. Cambios a la lógica de los tests.
2. Cambios a fixtures o helpers de tests.

## Estado inicial esperado

~32 callers de db.setup_schema() en tests, algunos con let db y otros con let mut db.

## Criterios de Aceptación

- cargo test --all-features compila sin errores de tipo relacionados con mutabilidad.
- Todos los tests de sqlite.rs pasan.

## Fuente de verdad

- src/storage/sqlite.rs (sección #[cfg(test)])
