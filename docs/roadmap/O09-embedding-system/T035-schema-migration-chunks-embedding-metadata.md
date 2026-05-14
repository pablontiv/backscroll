---
id: T035
tipo: task
estado: Completed
titulo: Schema migration — tablas chunks y embedding_metadata
outcome: O09
dependencias: [T034]
---

# T035 — Schema migration: tablas `chunks` + `embedding_metadata`

Nueva versión de migración en `internal/storage/migrations.go` que agrega las
tablas necesarias para el sistema de embeddings.

## Alcance

**Invariante del proyecto**: nueva tabla → nueva versión de migración. Nunca
modificar bloques existentes.

Agregar en `setupSchema()` un nuevo bloque `if currentVersion == N`:

```sql
-- chunks: fragmentos de texto indexados para embeddings
CREATE TABLE IF NOT EXISTS chunks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id TEXT NOT NULL,       -- FK a search_items.id o indexed_files.id
    chunk_idx INTEGER NOT NULL,    -- índice dentro de la sesión/documento
    content TEXT NOT NULL,
    token_count INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE(source_id, chunk_idx)
);

CREATE INDEX IF NOT EXISTS idx_chunks_source_id ON chunks (source_id);

-- embedding_metadata: metadatos del modelo usado para generar embeddings
CREATE TABLE IF NOT EXISTS embedding_metadata (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chunk_id INTEGER NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    model_version TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
```

## Criterios de aceptación

- `currentVersion` incrementado correctamente (verificar valor actual en `migrations.go`)
- Nueva DB incluye ambas tablas
- DB existente migra sin pérdida de datos (`go test ./internal/storage/...` pasa)
- El bloque anterior no fue modificado
- `go vet ./...` sin warnings

## Notas

- `vec_embeddings` se agrega en O10.T040, no aquí — mantener separados
- Verificar el valor actual de `currentVersion` en `migrations.go` antes de implementar
