---
id: T038
tipo: task
estado: Pending
titulo: ReciprocatRankFusion() en internal/hybrid/
outcome: O10
---

# T038 — `ReciprocatRankFusion()` en `internal/hybrid/`

Port de `reciprocal_rank_fusion()` de `src/core/hybrid.rs` (v0). Combina dos
rankings (BM25 y vector) en un ranking unificado usando el algoritmo RRF.

## Alcance

En `internal/hybrid/rrf.go`:

```go
// RankResult representa un resultado con su score.
type RankResult struct {
    ID    string
    Score float64
}

// ReciprocatRankFusion combina dos rankings usando RRF con constante k.
// RRF score = Σ 1/(k + rank_i) para cada lista que contiene el documento.
// k=60 es el valor estándar (Cormack et al. 2009).
func ReciprocatRankFusion(k int, rankings ...[]RankResult) []RankResult
```

## Criterios de aceptación

- Test: `rrf(60, [{A,1},{B,2},{C,3}], [{B,1},{A,2},{D,3}])` → B y A en los primeros
  dos puestos (B aparece top en segunda lista, A top en primera)
- Documentos que aparecen en solo una lista reciben score correcto
- Documentos en ambas listas reciben boost aditivo
- Resultado ordenado de mayor a menor score
- Test table-driven con casos del paper original
- `go test ./internal/hybrid/...` pasa

## Referencias

- `reciprocal_rank_fusion()` en `src/core/hybrid.rs` (v0 branch)
- Cormack et al. 2009 — RRF: k=60 como constante estándar
