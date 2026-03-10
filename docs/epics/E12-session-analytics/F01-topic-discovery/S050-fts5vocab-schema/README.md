# S050: fts5vocab schema & queries

**Feature**: [F01 Topic Discovery](../README.md)
**Capacidad**: Schema v3 crea tabla virtual fts5vocab sobre el indice FTS5 existente. Funciones de query retornan term frequencies con filtrado de stopwords y filtro por proyecto.
**Cubre**: P1 (topics retorna terminos rankeados)

## Antes / Despues

**Antes**: No hay forma de obtener estadisticas de terminos del indice FTS5. El skill hace multiples busquedas con keywords hardcoded como workaround.

**Despues**: `messages_vocab` (fts5vocab virtual table) expone term frequencies. `get_topics()` retorna terminos ordenados por document frequency, con stopwords filtrados y filtro opcional por proyecto via JOIN con search_items.

## Criterios de Aceptacion (semanticos)

- [ ] Schema migration v3 crea messages_vocab sin error
- [ ] fts5vocab retorna terms con doc count y total count
- [ ] Stopwords filtrados (el, la, de, que, en, the, is, a, to, etc.)
- [ ] Filtro por proyecto funciona via JOIN con search_items
- [ ] Migration es idempotente (re-run no falla)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T071](T071-schema-v3-fts5vocab.md) | Schema v3 migration — create fts5vocab virtual table |
| [T072](T072-topics-query-function.md) | Topics query function with stopwords + project filter |
| [T073](T073-test-fts5vocab.md) | Test fts5vocab term frequency output |

## Fuente de verdad

- `src/storage/sqlite.rs` — schema setup + query functions
