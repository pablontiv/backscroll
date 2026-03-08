---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T033: Modificar SQL para usar snippet()

**Story**: [S034 FTS5 snippet extraction](README.md)
**Contribuye a**: SearchResult.match_snippet contiene texto con markers

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Contexto

FTS5 tiene funcion auxiliar `snippet()` que extrae fragmentos de texto alrededor de los terminos de busqueda. Actualmente la query retorna el texto completo. Con external content FTS5, snippet() funciona nativamente (rusqlite lo soporta).

## Especificacion Tecnica

```sql
SELECT
    si.id, si.source_path, si.text, si.timestamp, si.uuid, si.project,
    snippet(messages_fts, 0, '>>>', '<<<', '...', 32) as snippet,
    rank as score
FROM messages_fts
JOIN search_items si ON si.id = messages_fts.rowid
WHERE messages_fts MATCH ?
ORDER BY rank
LIMIT 20
```

## Alcance

**In**:
1. Modificar query SQL en search() de sqlite.rs para incluir snippet()
2. Markers: `>>>` para start, `<<<` para end, `...` para ellipsis, 32 tokens max
3. Mapear resultado snippet a SearchResult.match_snippet

**Out**: No formatear para terminal (S035). Solo extraer raw snippet.

## Estado inicial esperado

- search() retorna SearchResult con match_snippet: None (T014)
- messages_fts es external content (T006)

## Criterios de Aceptacion

- `cargo test test_snippet_extraction` — snippet contiene `>>>` y `<<<` markers
- Snippet es fragmento (no texto completo)
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
