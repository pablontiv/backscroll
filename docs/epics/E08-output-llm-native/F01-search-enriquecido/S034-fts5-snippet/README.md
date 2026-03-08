# S034: FTS5 snippet extraction

**Feature**: [F01 Search Enriquecido](../README.md)
**Capacidad**: Queries FTS5 extraen snippets con highlight markers y los populan en SearchResult.
**Cubre**: P1 del Epic (output con snippet y highlight)

## Antes / Despues

**Antes**: Search retorna `SearchResult { path, content, score }` con el contenido completo del mensaje. No hay snippet ni highlight. Score se calcula pero no se muestra.

**Despues**: Query SQL usa `snippet(messages_fts, 0, '>>>', '<<<', '...', 32)` para extraer fragmentos relevantes. `SearchResult.match_snippet` pasa de `None` a `Some(snippet)`.

## Criterios de Aceptacion (semanticos)

- [x] SearchResult.match_snippet contiene texto con markers `>>>` y `<<<`
- [x] Snippet es un fragmento relevante, no el contenido completo

## Invariantes

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Tasks

| Task | Descripcion |
|------|-------------|
| [T033](T033-snippet-sql.md) | Modificar SQL para usar snippet() |
| [T034](T034-populate-snippet.md) | Popular match_snippet en SearchResult |

## Fuente de verdad

- `src/storage/sqlite.rs` — query SQL de search
- `src/core/mod.rs` — SearchResult struct
