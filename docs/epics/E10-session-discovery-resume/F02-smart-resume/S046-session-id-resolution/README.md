# S046: Session ID resolution

**Feature**: [F02 Smart Resume](../README.md)
**Capacidad**: Dado el source_path de un search result, resolver el session UUID que `claude --resume` espera.
**Cubre**: P3 del Epic (resume produce session ID)

[[blocks:S045-resume-subcommand]]

## Antes / Despues

**Antes**: `SearchResult` tiene `source_path` pero no session ID. Session UUIDs estan en `search_items.uuid` pero son per-message, no per-session.

**Despues**: `SearchEngine` trait tiene `get_session_id(source_path) -> Option<String>` que retorna el primer UUID para ese archivo. El UUID del primer record del JSONL es el identificador de sesion.

## Criterios de Aceptacion (semanticos)

- [ ] Session ID se extrae del UUID del primer record en el JSONL
- [ ] Si no se encuentra UUID, fallback a file stem como identificador
- [ ] Funciona con ambos layouts de directorio (legacy y actual)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T060](T060-get-session-id-trait.md) | Add get_session_id to SearchEngine |
| [T061](T061-tests-session-id.md) | Tests session ID resolution |

## Fuente de verdad

- `src/core/mod.rs` — SearchEngine trait
- `src/storage/sqlite.rs` — Database impl
