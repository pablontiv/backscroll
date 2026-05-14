---
id: T033
tipo: task
estado: Completed
titulo: OnnxProvider — evaluar hugot vs CGO e implementar
outcome: O09
dependencias: [T031]
---

# T033 — `OnnxProvider`: evaluar hugot vs CGO e implementar

Implementar el provider de embeddings usando un modelo ONNX (all-MiniLM-L6-v2
o equivalente, 384 dimensiones). Decisión clave: pure Go vs CGO.

## Evaluación requerida (documentar decisión)

| Opción | Pros | Contras |
|---|---|---|
| `github.com/knights-analytics/hugot` | Pure Go, sin CGO, cross-compilable | Más nuevo, menos battle-tested |
| `github.com/yalue/onnxruntime_go` | Más maduro, bindings directos | Requiere CGO + runtime nativo |
| Pure Go ONNX (implementar subset) | Sin dependencias externas | Altísimo esfuerzo |

**Criterio de decisión**: mantener el principio "no CGO si evitable" del proyecto.
Si hugot soporta all-MiniLM-L6-v2 con pooling mean, usar hugot.

## Alcance

En `internal/embedding/onnx_provider.go`:

```go
type OnnxProvider struct { ... }

func NewOnnxProvider(modelPath string) (*OnnxProvider, error)
func (p *OnnxProvider) Embed(ctx context.Context, text string) ([]float32, error)
func (p *OnnxProvider) Dimensions() int   // 384
func (p *OnnxProvider) Close() error
```

- Descarga del modelo si `model_path` no existe (HuggingFace Hub o URL configurable)
- Pooling: mean pooling sobre los token embeddings (all-MiniLM-L6-v2 style)
- Normalización L2 del vector de salida

## Criterios de aceptación

- `OnnxProvider` compila sin CGO (si hugot) o con build tag opcional (si CGO)
- `Embed("hello world")` retorna vector de 384 floats
- Test de similitud coseno: `cosine("cat", "dog") > cosine("cat", "database")`
- La decisión hugot vs CGO está documentada en un ADR o en el commit message
- `go test ./internal/embedding/...` pasa (con modelo descargado en CI)

## Notas de CI

- Si el modelo es pesado (>50MB), usar `MockEmbeddingProvider` en tests unitarios
- Tests de integración con ONNX bajo build tag `integration`
