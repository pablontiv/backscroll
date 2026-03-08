---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T006: Crear messages_fts external content + triggers

**Story**: [S022 Schema versioning + content table](README.md)
**Contribuye a**: FTS5 usa external content pattern con triggers

[[blocks:T005-search-items-table]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El FTS5 actual es inline (datos duplicados en la tabla virtual). Se migra a external content pattern donde `messages_fts` apunta a `search_items` como content table, con triggers para mantener sincronizacion.

## Especificacion Tecnica

```sql
-- Drop old inline FTS5
DROP TABLE IF EXISTS messages_fts;

-- Create external content FTS5
CREATE VIRTUAL TABLE messages_fts USING fts5(
    text,
    content=search_items,
    content_rowid=id,
    tokenize='unicode61'
);

-- Triggers para sincronizacion
CREATE TRIGGER search_items_ai AFTER INSERT ON search_items BEGIN
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;

CREATE TRIGGER search_items_ad AFTER DELETE ON search_items BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
END;

CREATE TRIGGER search_items_au AFTER UPDATE ON search_items BEGIN
    INSERT INTO messages_fts(messages_fts, rowid, text) VALUES('delete', old.id, old.text);
    INSERT INTO messages_fts(rowid, text) VALUES (new.id, new.text);
END;
```

## Alcance

**In**:
1. Agregar DDL de FTS5 external content como parte de migracion v2
2. Crear 3 triggers (AI, AD, AU) para sincronizacion automatica
3. Drop de la tabla FTS5 inline anterior
4. Test que verifica que INSERT en search_items popula messages_fts automaticamente

**Out**: No modificar queries de search (se adaptan en S025/S026).

## Estado inicial esperado

- search_items table existe (T005 completado)
- messages_fts inline existe (sera dropeada)

## Criterios de Aceptacion

- `sqlite3 test.db ".schema messages_fts"` muestra `content=search_items`
- INSERT en search_items automaticamente popula messages_fts (verificar con SELECT)
- DELETE de search_items automaticamente limpia messages_fts
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
