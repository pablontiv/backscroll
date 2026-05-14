---
id: T041
tipo: task
estado: Obsolete
titulo: Vector search en internal/storage/
outcome: O10
dependencias: [T040, T031]
---

# T041 — Vector search en `internal/storage/`

Implementar las operaciones de almacenamiento y recuperación de vectores, y la
función de búsqueda por similitud coseno.

## Alcance

En `internal/storage/` (o nuevo archivo `vector.go`):

```go
// StoreEmbedding guarda un vector para un chunk dado.
func (s *Storage) StoreEmbedding(ctx context.Context, chunkID int64, vec []float32) error

// SearchSimilar retorna los top-K chunks más similares al vector dado.
// threshold: similitud coseno mínima (0.0 = cualquiera, 1.0 = exacto).
func (s *Storage) SearchSimilar(ctx context.Context, vec []float32, topK int, threshold float32) ([]models.SearchResult, error)
```

Si sqlite-vec: delegar a la virtual table.
Si fallback: linear scan sobre `vec_embeddings` con cosine similarity Go-side.

## Criterios de aceptación

- `StoreEmbedding` + `SearchSimilar` funcionales en test con MockProvider
- Cosine similarity correcta: `sim([1,0],[1,0]) == 1.0`, `sim([1,0],[0,1]) == 0.0`
- `SearchSimilar` retorna resultados ordenados por similitud descendente
- `threshold` filtra correctamente
- `go test ./internal/storage/...` pasa
