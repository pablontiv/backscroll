---
id: O13
tipo: outcome
estado: Pending
titulo: CLI parity gaps vs v0 Rust
descripcion: Cerrar las brechas funcionales restantes entre el Go port y el v0 Rust — flags faltantes en events/sessions/list/status, subcomando inputs validate ausente, y rename de --no-embed a --no-embeddings.
---

# O13 — CLI parity gaps vs v0 Rust

Gaps funcionales confirmados por auditoría directa de `git show v0:src/main.rs` vs Go port.

## Criterios de éxito

- CE1: `backscroll inputs validate [--json]` funciona y valida manifests sin sync
- CE2: `backscroll events query` soporta `--project`, `--all-projects`, `--source`, `--source-path`, `--event-type`, `--indexed-only`
- CE3: `backscroll sessions query` soporta `--source-path`, `--max-chars`, `--indexed-only`
- CE4: `backscroll list --recent 20` muestra 20 sesiones más recientes (int, no bool)
- CE5: `--indexed-only` disponible en `list` y `status`
- CE6: `--no-embeddings` en `sync` y `reindex` (renombrado de `--no-embed`)
- CE7: `go test ./...` pasa con coverage ≥85%

## Invariantes

- INV1: No CGO
- INV2: Schema migration rule — ninguna tabla/columna nueva sin nueva versión de migración
- INV3: Todo cambio respaldado por task del roadmap

## Tasks

- T050: `inputs validate` subcomando
- T051: `events query` filtros faltantes
- T052: `sessions query` filtros faltantes
- T053: `list --recent N` (int) + `--indexed-only` en list y status
- T054: rename `--no-embed` → `--no-embeddings` en sync y reindex
