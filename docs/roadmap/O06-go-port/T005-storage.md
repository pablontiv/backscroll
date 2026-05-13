---
estado: Completed
tipo: task
---
# T005: Storage — SQLite FTS5

**Outcome**: [Port a Go](README.md)

## Contexto

El módulo más denso del port. Equivalente a `storage/sqlite.rs` (2.948 líneas) + `storage/migrations.rs`. Usa `modernc.org/sqlite` (puro Go, sin CGO) que soporta FTS5, WAL mode y triggers. Sin sqlite-vec ni embeddings.

## Alcance

**In**:
1. `internal/storage` — Database struct, open/open-readonly, migrations versioned (schema compatible con el actual para no forzar reindex).
2. FTS5 con Porter stemmer tokenizer, tabla `search_items`, triggers, `snippet()`.
3. BM25 search con todos los filtros actuales: `--project`, `--all-projects`, `--source`, `--after`, `--before`, `--role`, `--limit`, `--offset`, `--content-type`, `--tag`.
4. Escritura: `sync_files()`, dedup por SHA-256, `session_tags`, `session_events`.
5. `SearchEngine` interface (equivalente al trait Rust) como port para el adapter.
6. Tests con DB real (sin mocks): fresh DB, idempotencia de migrations, queries BM25.

**Out**:
1. Embeddings y sqlite-vec (eliminados permanentemente en este port).
2. Comandos CLI (van en T007).

## Criterios de Aceptación

- La DB existente del usuario (`~/.backscroll.db`) abre sin error y `backscroll status` reporta el mismo conteo que la versión Rust.
- BM25 search retorna resultados para queries conocidas con `--source session`.
- Migrations son idempotentes: correr dos veces no produce error ni duplicados.
- `go test ./internal/storage/...` pasa con DB real (usando `tempfile`).
