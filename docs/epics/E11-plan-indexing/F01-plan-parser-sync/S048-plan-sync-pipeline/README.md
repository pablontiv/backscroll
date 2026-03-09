# S048: Plan sync pipeline

**Feature**: [F01 Plan Parser & Sync](../README.md)
**Capacidad**: Sync command descubre `~/.claude/plans/`, parsea todos los `.md` files, y los sincroniza en `search_items` con `source='plan'` y SHA-256 dedup.
**Cubre**: P1 del Epic (plans indexados)

[[blocks:S047-markdown-plan-parser]]

## Antes / Despues

**Antes**: `sync_files()` solo procesa archivos JSONL de sesion. La columna `source` siempre es 'session'.

**Despues**: `sync_files()` acepta `ParsedFile` con campo `source`. Plans se sincronizan con `source='plan'`. `ParsedFile` gana campo `source: String`.

## Criterios de Aceptacion (semanticos)

- [ ] Plan sync usa mismo SHA-256 dedup que sessions
- [ ] Re-ejecutar sync omite plan files sin cambios
- [ ] `ParsedFile` tiene campo `source`, defaulting a "session" para backward compat
- [ ] Plans directory auto-descubierto de `~/.claude/plans/`

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV4: Sync incremental funciona para plans
  - Verificar: segundo sync no re-procesa plans sin cambios

## Tasks

| Task | Descripcion |
|------|-------------|
| [T064](T064-source-field-parsedfile.md) | Add source field to ParsedFile |
| [T065](T065-plan-sync-main.md) | Plan sync in main.rs |
| [T066](T066-plan-sync-integration-test.md) | Plan sync integration test |

## Fuente de verdad

- `src/core/mod.rs` — ParsedFile struct
- `src/storage/sqlite.rs` — sync_files() INSERT
- `src/main.rs` — sync dispatch
