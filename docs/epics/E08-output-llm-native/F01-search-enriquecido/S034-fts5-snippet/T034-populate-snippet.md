---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T034: Popular match_snippet en SearchResult

**Story**: [S034 FTS5 snippet extraction](README.md)
**Contribuye a**: Snippet es un fragmento relevante, no el contenido completo

[[blocks:T033-snippet-sql]]

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Contexto

Conectar el snippet extraido por SQL con el campo match_snippet de SearchResult. Verificar end-to-end que el flujo funciona: query → FTS5 snippet → SearchResult → disponible para output.

## Alcance

**In**:
1. Asegurar que row.get("snippet") se mapea a SearchResult.match_snippet
2. Test end-to-end: index → search → SearchResult tiene snippet con markers
3. Verificar que snippet no es None cuando hay match

**Out**: No formatear para output (S035).

## Estado inicial esperado

- SQL con snippet() implementado (T033)
- SearchResult tiene campo match_snippet (T012/T014)

## Criterios de Aceptacion

- `cargo test test_search_result_has_snippet` — match_snippet es Some
- Snippet contiene termino de busqueda entre markers
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
- `src/core/mod.rs`
