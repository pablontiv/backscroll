---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T072: Topics query function with stopwords + project filter

**Story**: [S050 fts5vocab schema & queries](README.md)
**Contribuye a**: P1 (topics retorna terminos rankeados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Need a function that queries fts5vocab for top terms by document frequency, with stopword filtering and optional project filter.

## Especificacion Tecnica

Add `get_topics()` to Database impl. Stopwords list as const array. Project filter via subquery on search_items.

Basic query:
```sql
SELECT term, doc, cnt FROM messages_vocab
WHERE length(term) > 3 AND term NOT IN (...stopwords)
ORDER BY doc DESC LIMIT ?
```

Project filter:
```sql
SELECT term, COUNT(DISTINCT si.source_path) as doc, COUNT(*) as cnt
FROM messages_vocab mv
JOIN search_items si ON si.text LIKE '%' || mv.term || '%'
WHERE si.project = ? ...
GROUP BY term ORDER BY doc DESC LIMIT ?
```

Or better: use the FTS5 content table relationship.

## Alcance

**In**: Add `get_topics()` to Database impl. Stopwords list as const array. Project filter via subquery on search_items.
**Out**: No CLI subcommand (T074), no output formatting (T075)

## Criterios de Aceptacion

- `get_topics(None, 30)` returns top 30 terms
- `get_topics(Some("myproject"), 10)` filters by project
- Stopwords excluded
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`, `src/core/mod.rs` (SearchEngine trait)
