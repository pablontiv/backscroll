---
id: T044
tipo: task
estado: Obsolete
titulo: Tests de búsqueda híbrida (mock embeddings)
outcome: O10
dependencias: [T042, T043]
---

# T044 — Tests de búsqueda híbrida (mock embeddings)

Tests unitarios e integración para el pipeline híbrido completo, usando
`MockEmbeddingProvider` para no requerir ONNX en CI.

## Alcance

- `internal/hybrid/rrf_test.go`: tests table-driven de `ReciprocatRankFusion`
- `internal/storage/vector_test.go`: `StoreEmbedding` + `SearchSimilar` con MockProvider
- `cmd/backscroll/main_test.go`: tests de integración para flags `--lexical_only`,
  `--similarity_threshold`, `--vector_only`
- Test de regresión: `backscroll search "query"` sin embeddings habilitados
  produce resultado idéntico al comportamiento actual

## Criterios de aceptación

- `go test -race ./...` pasa sin data races
- `just coverage` ≥85% en todos los packages afectados
- Tests son herméticos: no requieren ONNX, no tocan `~/.config/`
- `--lexical_only` produce resultados idénticos al search actual (test de regresión)
