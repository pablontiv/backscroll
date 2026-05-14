---
id: O10
tipo: outcome
estado: Completed
titulo: Hybrid Search (BM25 + Vector + RRF) en Go
descripcion: Port de src/core/hybrid.rs. Combina búsqueda lexical FTS5/BM25 con búsqueda vectorial via sqlite-vec usando Reciprocal Rank Fusion. Requiere O09 (embeddings).
---

# O10 — Hybrid Search (BM25 + Vector + RRF) en Go

Port de `src/core/hybrid.rs` del branch v0. Combina los resultados de BM25 (FTS5)
y búsqueda vectorial (sqlite-vec) usando Reciprocal Rank Fusion para ranking híbrido.

**Depende de**: O09 (sistema de embeddings completo)

## Tasks

- [T038](T038-reciprocal-rank-fusion.md) — `ReciprocatRankFusion()` en `internal/hybrid/`
- [T039](T039-sqlite-vec-integracion.md) — Evaluar e integrar sqlite-vec
- [T040](T040-schema-migration-vec-embeddings.md) — Schema migration: `vec_embeddings` virtual table
- [T041](T041-vector-search-storage.md) — Vector search en `internal/storage/`
- [T042](T042-hybrid-search-command.md) — Wire hybrid search en comando `search`
- [T043](T043-stats-embedding-count.md) — `get_stats()` / `status` con embedding count
- [T044](T044-tests-hybrid-search.md) — Tests de búsqueda híbrida

## Criterios de cierre

- `backscroll search "query"` usa hybrid BM25+vector cuando embeddings habilitados
- `backscroll search "query" --lexical_only` usa solo BM25 (comportamiento actual)
- `backscroll search "query" --similarity_threshold 0.7` filtra por similitud mínima
- `backscroll status` incluye `embeddings: N`
- `go test ./...` pasa con coverage ≥85%
