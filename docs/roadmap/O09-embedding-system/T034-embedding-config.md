---
id: T034
tipo: task
estado: Completed
titulo: EmbeddingConfig en config structs
outcome: O09
dependencias: [T031]
---

# T034 — `EmbeddingConfig` en config structs

Agregar la sección `[embedding]` en `backscroll.toml` y los structs Go correspondientes.

## Alcance

En `internal/config/config.go`, agregar:

```go
type EmbeddingConfig struct {
    Enabled             bool    `toml:"enabled"`
    ModelName           string  `toml:"model_name"`    // e.g., "all-MiniLM-L6-v2"
    ModelPath           string  `toml:"model_path"`    // path local o "" para auto-download
    SimilarityThreshold float32 `toml:"similarity_threshold"` // default: 0.7
    TopK                int     `toml:"top_k"`         // default: 10
}
```

- `Config.Embedding EmbeddingConfig` — nuevo campo en el struct principal
- `Embedding.Enabled = false` por defecto (feature opt-in)
- Defaults aplicados en `LoadConfig()` si no está en el TOML
- Documentar la sección `[embedding]` en `backscroll.toml.example` si existe

## Criterios de aceptación

- `backscroll.toml` existente sin `[embedding]` no rompe nada (defaults)
- `backscroll.toml` con `[embedding] enabled = true, model_name = "all-MiniLM-L6-v2"`
  se parsea correctamente
- `go test ./internal/config/...` pasa
- `go test ./...` pasa (no regresiones)
