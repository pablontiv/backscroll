---
dependencias: '- T041'
estado: Completed
id: T043
outcome: O10
tipo: task
titulo: get_stats() / status con embedding count
---

# T043 — `get_stats()` / `status` con embedding count

Actualizar el comando `backscroll status` para incluir estadísticas del sistema
de embeddings: cantidad de chunks, embeddings generados, y modelo activo.

## Alcance

En `internal/storage/` — función `GetStats()` o equivalente:

```go
type Stats struct {
    // ... campos existentes ...
    ChunksCount     int64  `json:"chunks_count"`
    EmbeddingsCount int64  `json:"embeddings_count"`
    EmbeddingModel  string `json:"embedding_model,omitempty"`
    AvgChunkTokens  float64 `json:"avg_chunk_tokens,omitempty"`
}
```

En `cmd/backscroll/status.go`: mostrar los nuevos campos en output text y JSON.

Output text:
```
chunks: 1234 (avg 287 tokens)
embeddings: 1234 (model: all-MiniLM-L6-v2)
```

Output JSON: campos adicionales en el objeto existente.

## Criterios de aceptación

- `backscroll status` muestra chunks y embeddings cuando existen
- `backscroll status --json` incluye `chunks_count`, `embeddings_count`, `embedding_model`
- Sin embeddings en DB: campos muestran 0 / omitidos en JSON
- `go test ./cmd/backscroll/...` pasa (tests de integración de status)
