---
id: O11
tipo: outcome
estado: Pending
titulo: CLI Subcomandos Faltantes en Go
descripcion: Port de los subcomandos ausentes en el Go port respecto a v0 Rust — events query, sessions namespace, status mejorado con embedding stats. Nota: dynamic_stopwords y FTS5 vocab ya están implementados en migrations.go.
---

# O11 — CLI Subcomandos Faltantes en Go

Los subcomandos que existían en v0 y no están en el Go port. Excluye
`dynamic_stopwords`/`messages_fts`/`messages_vocab` que ya están en
`internal/storage/migrations.go`.

**Depende de**: O08 (session events extraction), O10 (embedding stats para T047)

## Tasks

- [T045](T045-events-query-subcommand.md) — `events query` subcommand
- [T046](T046-sessions-subcommands.md) — `sessions query/list/validate` namespace
- [T047](T047-status-json-embedding-stats.md) — Mejora `status --json` con embedding stats e inputs activos
- [T048](T048-tests-integracion-subcomandos.md) — Tests de integración para nuevos subcomandos

## Criterios de cierre

- `backscroll events query <session-id>` emite JSONL de mensajes individuales
- `backscroll sessions list` lista sesiones indexadas
- `backscroll sessions validate` valida integridad de sesiones
- `backscroll status --json` incluye `embeddings_count`, `active_inputs`
- `go test ./...` pasa con coverage ≥85%
