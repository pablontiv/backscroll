---
id: T040
tipo: task
estado: Obsolete
titulo: Schema migration — vec_embeddings virtual table (384-dim)
outcome: O10
dependencias: [T039, T035]
---

# T040 — Schema migration: `vec_embeddings` virtual table

Nueva versión de migración que agrega la tabla virtual `vec_embeddings` para
búsqueda de similitud coseno sobre vectores de 384 dimensiones.

## Alcance

Nuevo bloque `if currentVersion == N` en `setupSchema()`:

Si sqlite-vec está disponible (resultado de T039):
```sql
CREATE VIRTUAL TABLE IF NOT EXISTS vec_embeddings USING vec0(
    chunk_id INTEGER PRIMARY KEY,
    embedding float[384]
);
```

Si fallback pure Go (`VecIndex`): la "tabla" se implementa como tabla regular
con `embedding BLOB NOT NULL` + índice Go-side:
```sql
CREATE TABLE IF NOT EXISTS vec_embeddings (
    chunk_id INTEGER PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
    embedding BLOB NOT NULL,     -- float32 array serializado
    created_at INTEGER NOT NULL
);
```

## Criterios de aceptación

- Migración correctamente versionada (currentVersion += 1)
- Tabla creada en DB nueva
- DB existente migra sin pérdida
- `INSERT` de vector de 384 floats funciona
- `go test ./internal/storage/...` pasa
