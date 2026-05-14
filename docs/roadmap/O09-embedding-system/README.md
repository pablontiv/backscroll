---
id: O09
tipo: outcome
estado: Pending
titulo: Embedding System en Go
descripcion: Port del sistema de embeddings vectoriales (EmbeddingProvider trait, OnnxProvider, ChunkText) del v0 Rust al Go port. Habilita búsqueda semántica y es prerequisito de O10.
---

# O09 — Embedding System en Go

Port de `src/core/embedding.rs` y `src/core/chunking.rs` del branch v0.
Introduce embeddings vectoriales 384-dim para búsqueda semántica, chunking
token-aware, y las migraciones de schema para `chunks` + `embedding_metadata`.

**Depende de**: O07 (config structs), O08 (pipeline de sync)

## Tasks

- [T031](T031-embedding-provider-interface.md) — `EmbeddingProvider` interface + `MockEmbeddingProvider`
- [T032](T032-chunk-text.md) — `ChunkText()` token-aware en `internal/chunking/`
- [T033](T033-onnx-provider.md) — `OnnxProvider`: evaluar hugot vs CGO e implementar
- [T034](T034-embedding-config.md) — `EmbeddingConfig` en config structs
- [T035](T035-schema-migration-chunks-embedding-metadata.md) — Schema migration: `chunks` + `embedding_metadata`
- [T036](T036-wiring-embeddings-sync.md) — Wiring embeddings en sync pipeline
- [T037](T037-tests-embedding-system.md) — Tests del sistema de embeddings

## Criterios de cierre

- `backscroll sync` genera embeddings para contenido nuevo (con provider ONNX configurado)
- `backscroll status` muestra `embeddings: N`
- `go test ./...` pasa con coverage ≥85%
- `MockEmbeddingProvider` permite tests sin ONNX runtime
