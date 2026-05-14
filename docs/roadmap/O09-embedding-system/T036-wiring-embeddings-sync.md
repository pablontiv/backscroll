---
id: T036
tipo: task
estado: Pending
titulo: Wiring embeddings en sync pipeline
outcome: O09
dependencias: [T032, T033, T035, T029]
---

# T036 — Wiring embeddings en sync pipeline

Integrar el sistema de embeddings en el pipeline de sync (post-O08). Cuando
`embedding.enabled = true`, cada sesión nueva es chunkeada y embedeada.

## Alcance

En `internal/sync/sync.go` o equivalente post-refactor (T029):

```
Para cada ParsedFile sincronizado (si embedding.enabled):
  1. ChunkText(content, maxTokens=512, overlap=50) → []string
  2. Para cada chunk: EmbeddingProvider.Embed(chunk) → []float32
  3. Guardar en chunks + embedding_metadata tables
  4. Guardar vector en vec_embeddings (O10.T040 — skip si no disponible)
```

- Solo embedear contenido nuevo (dedup por chunk hash)
- Progreso: loggear chunks procesados / total
- Si el provider falla, loggear warning y continuar (no abortar sync)
- `--no-embed` flag en `backscroll sync` para deshabilitar embeddings en esta ejecución

## Criterios de aceptación

- `backscroll sync` con `embedding.enabled = true` guarda chunks en DB
- `backscroll status` muestra `chunks: N`, `embeddings: N`
- Sin `embedding.enabled`, el sync funciona exactamente como hoy
- Error del provider ONNX no aborta el sync completo
- `go test ./...` pasa con `MockEmbeddingProvider` (no requiere ONNX real en CI)
