---
id: T032
tipo: task
estado: Pending
titulo: ChunkText() token-aware en internal/chunking/
outcome: O09
---

# T032 — `ChunkText()` token-aware en `internal/chunking/`

Port de `chunk_text()` de `src/core/chunking.rs` (v0). Divide texto largo en
chunks de tamaño máximo en tokens, respetando límites de párrafo → oración → palabra.

## Alcance

En `internal/chunking/chunking.go`:

```go
// ChunkText divide text en chunks de a lo sumo maxTokens tokens.
// Respeta límites de párrafo → oración → palabra en ese orden de preferencia.
// overlap: número de tokens de contexto solapado entre chunks consecutivos.
func ChunkText(text string, maxTokens int, overlap int) []string
```

Estrategia de tokenización: aproximación por palabras (1 token ≈ 0.75 palabras,
o usar un tokenizador simple BPE-compatible si está disponible en pure Go).
La precisión exacta no es crítica — el objetivo es evitar chunks de >512 tokens.

## Criterios de aceptación

- Texto de 2000 palabras con `maxTokens=512` produce ≥3 chunks
- Cada chunk tiene ≤512 tokens (aprox)
- Los chunks se solapan en `overlap` tokens cuando se especifica
- No corta palabras a la mitad
- Preferencia: cortar en límite de párrafo > oración > palabra
- Test table-driven con textos de distintos tamaños
- `go test ./internal/chunking/...` pasa

## Referencias

- `chunk_text()` en `src/core/chunking.rs` (v0 branch)
