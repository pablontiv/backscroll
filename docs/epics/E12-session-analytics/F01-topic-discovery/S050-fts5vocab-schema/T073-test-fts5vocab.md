---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T073: Test fts5vocab term frequency output

**Story**: [S050 fts5vocab schema & queries](README.md)
**Contribuye a**: P1 (topics retorna terminos rankeados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Verify fts5vocab returns correct term frequencies after indexing test data.

## Especificacion Tecnica

Integration test: sync fixture data, query topics, verify term counts.

1. Create test database with known fixture data
2. Sync fixture with known terms
3. Call `get_topics()` and verify top terms match expected
4. Test with project filter returns only project-specific terms

## Alcance

**In**: Integration test for fts5vocab and get_topics()
**Out**: No CLI testing (T076)

## Criterios de Aceptacion

- Test syncs fixture with known terms, verifies top terms match expected
- Test with project filter returns only project-specific terms
- `just test` pasa

## Fuente de verdad

- `src/storage/sqlite.rs` — mod tests
