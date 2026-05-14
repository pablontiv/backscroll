---
id: T037
tipo: task
estado: Completed
titulo: Tests del sistema de embeddings (mock provider)
outcome: O09
dependencias: [T036]
---

# T037 — Tests del sistema de embeddings (mock provider)

Tests unitarios y de integración para el sistema de embeddings completo, usando
`MockEmbeddingProvider` para no depender del runtime ONNX en CI.

## Alcance

- `internal/embedding/mock_test.go`: test de `MockEmbeddingProvider` (embed, dimensions, close)
- `internal/chunking/chunking_test.go`: table-driven tests de `ChunkText`
- `internal/storage/embedding_test.go`: test de guardado/recuperación de chunks en DB
- `internal/sync/embed_sync_test.go`: test de sync con embeddings (mock provider)
  - Fixture JSONL → sync → verificar que chunks se guardaron
  - Segundo sync mismo archivo → 0 chunks nuevos (dedup)

## Criterios de aceptación

- `go test -race ./...` pasa sin data races
- `just coverage` ≥85% en todos los packages afectados
- Tests son herméticos: no tocan `~/.config/`, no necesitan ONNX descargado
- Build tag `integration` para tests que requieren ONNX real (excluidos de CI normal)
