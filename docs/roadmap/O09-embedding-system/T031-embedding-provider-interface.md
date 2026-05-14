---
id: T031
tipo: task
estado: Pending
titulo: EmbeddingProvider interface + MockEmbeddingProvider
outcome: O09
---

# T031 — `EmbeddingProvider` interface + `MockEmbeddingProvider`

Definir la abstracción central del sistema de embeddings. Análogo al trait
`EmbeddingProvider` en `src/core/embedding.rs` (v0).

## Alcance

En `internal/embedding/provider.go`:

```go
// EmbeddingProvider genera embeddings vectoriales para texto.
type EmbeddingProvider interface {
    // Embed genera un vector de dimensión fija para el texto dado.
    Embed(ctx context.Context, text string) ([]float32, error)
    // Dimensions retorna la dimensión del vector de salida (e.g., 384).
    Dimensions() int
    // Close libera los recursos del provider.
    Close() error
}
```

En `internal/embedding/mock.go`:

```go
// MockEmbeddingProvider retorna vectores deterministas para testing.
type MockEmbeddingProvider struct {
    dims int
}

func NewMockProvider(dims int) *MockEmbeddingProvider
```

El mock retorna un vector de zeros excepto las primeras `n` posiciones que reflejan
el hash del texto (para que textos distintos den vectores distintos en tests).

## Criterios de aceptación

- Interface compilable
- `MockEmbeddingProvider` implementa `EmbeddingProvider`
- `mock.Embed("hello") != mock.Embed("world")` (vectores distintos)
- `mock.Dimensions() == dims` (configurable)
- `go test ./internal/embedding/...` pasa

## Referencias

- `trait EmbeddingProvider` en `src/core/embedding.rs` (v0 branch)
