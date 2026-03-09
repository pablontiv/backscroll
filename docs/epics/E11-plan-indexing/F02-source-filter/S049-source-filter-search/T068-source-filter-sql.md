---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T068: Source filter in SQL

**Story**: [S049 Source filter in search](README.md)
**Contribuye a**: P3 (source filter), P4 (default preserva comportamiento)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Busqueda sin --source incluye todo
  - Verificar: default "all" no filtra

## Contexto

SearchEngine::search() necesita aceptar filtro de source opcional.

## Especificacion Tecnica

1. En `src/core/mod.rs`, cambiar signature:
   ```rust
   fn search(&self, query: &str, project: &Option<String>, source: &Option<String>) -> miette::Result<Vec<SearchResult>>;
   ```

2. En `src/storage/sqlite.rs`, agregar clausula condicional:
   - Si source es Some("sessions") → `AND si.source = 'session'`
   - Si source es Some("plans") → `AND si.source = 'plan'`
   - Si source es None o Some("all") → sin filtro adicional

3. Actualizar todos los call sites de search() para pasar el nuevo parametro

## Alcance

**In**: Trait signature, SQL implementation, call sites
**Out**: No agregar flag CLI (T067)

## Criterios de Aceptacion

- source="sessions" filtra por session
- source="plans" filtra por plan
- source="all" o None no filtra
- Call sites existentes actualizados

## Fuente de verdad

- `src/core/mod.rs` — SearchEngine trait
- `src/storage/sqlite.rs` — search() SQL
