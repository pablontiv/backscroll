---
id: T054
tipo: task
estado: Pending
titulo: Rename --no-embed → --no-embeddings en sync y reindex
outcome: O13
dependencias: []
---

# T054 — Rename `--no-embed` → `--no-embeddings`

Renombrar el flag `--no-embed` a `--no-embeddings` en `backscroll sync` y `backscroll reindex` para paridad con v0 Rust.

## Alcance

En `cmd/backscroll/sync.go`:
- Cambiar `cmd.Flags().BoolVar(&noEmbed, "no-embed", ...)` → `"no-embeddings"`
- Variable interna puede permanecer `noEmbed`

En `cmd/backscroll/reindex.go`:
- Si `reindex` llama `runSync`, verificar que pasa el valor correcto
- Si `reindex` tiene su propio flag `--no-embed`, renombrarlo también

## Criterios de aceptación

- `backscroll sync --no-embeddings` funciona (skip embedding pipeline)
- `backscroll sync --no-embed` da error de flag desconocido
- `backscroll sync --help` muestra `--no-embeddings`
- `go test ./cmd/backscroll/...` pasa
- Coverage ≥85% mantenido
