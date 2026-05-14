---
id: T042
tipo: task
estado: Obsolete
titulo: Wire hybrid search en comando search
outcome: O10
dependencias: [T041, T038, T034]
---

# T042 — Wire hybrid search en comando `search`

Integrar el pipeline BM25+vector+RRF en el comando `backscroll search` existente.
Cuando embeddings están habilitados y el query tiene representación vectorial,
combinar resultados BM25 y vectoriales via RRF.

## Alcance

En `cmd/backscroll/search.go`:

```
1. Obtener resultados BM25 via Search() existente → []RankResult
2. Si embedding.enabled:
   a. EmbeddingProvider.Embed(query) → queryVec
   b. SearchSimilar(queryVec, topK, threshold) → []RankResult
   c. ReciprocatRankFusion(60, bm25Results, vecResults) → híbrido
3. Si no: solo BM25 (comportamiento actual)
```

Nuevos flags en `backscroll search`:
- `--lexical_only`: forzar solo BM25 (ignorar embeddings aunque estén habilitados)
- `--similarity_threshold <float>`: override del threshold configurado (default: 0.7)
- `--vector_only`: solo búsqueda vectorial (sin BM25)

## Criterios de aceptación

- `backscroll search "query"` sin flags → hybrid si embeddings habilitados, BM25 si no
- `backscroll search "query" --lexical_only` → siempre BM25 puro
- `backscroll search "query" --similarity_threshold 0.5` → override efectivo
- `backscroll search "query" --json` sigue produciendo JSON válido con nuevo modo
- Tests de integración en `cmd/backscroll/main_test.go` para los nuevos flags
- `go test ./...` pasa
