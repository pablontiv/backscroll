---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T071: Schema v3 migration — create fts5vocab virtual table

**Story**: [S050 fts5vocab schema & queries](README.md)
**Contribuye a**: P1 (topics retorna terminos rankeados)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Backscroll uses SQLite FTS5 with schema v2 (external content table). Need to add fts5vocab virtual table in a v3 migration.

## Especificacion Tecnica

Add v2->v3 migration in `setup_schema()` that creates `messages_vocab` virtual table. Add version bump.

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS messages_vocab USING fts5vocab(messages_fts, row);
```

This exposes term/doc/cnt columns from the FTS5 index. No re-indexation needed.

## Alcance

**In**: Add v2->v3 migration in `setup_schema()` that creates `messages_vocab` virtual table. Add version bump.
**Out**: No query functions (T072), no tests (T073)

## Criterios de Aceptacion

- Schema v3 creates messages_vocab
- `SELECT term, doc, cnt FROM messages_vocab LIMIT 1` works after migration
- Existing v2 databases migrate cleanly
- `just test` pasa

## Fuente de verdad

- `src/storage/sqlite.rs` lines 76-150 (current v1->v2 migration pattern)
