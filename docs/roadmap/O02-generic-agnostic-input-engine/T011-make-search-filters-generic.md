---
estado: Specified
tipo: task
---
# T011: Make downstream source, role and content filtering generic

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE3

[[blocked_by:./T008-refactor-sync-read-api-to-input-engine.md]]

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: list/insights de sesiones siguen filtrando por `source = "session"` cuando corresponde.
- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: cambios son query/filter layer, no modelo de ingestion.

## Contexto

El engine genérico expondrá más sources y roles. Hoy hay hardcoding y bugs potenciales: `--source` mapea solo sessions/plans, role aliases están en storage/CLI, y el path híbrido puede no aplicar filtros igual que BM25.

## Alcance

**In**:
1. Hacer `--source` filtrar cualquier source real, no solo sessions/plans.
2. Separar role aliases configurables o documentados del core de parser.
3. Aplicar filtros consistentemente en BM25 y vector/hybrid.
4. Revisar content type validation para no asumir solo semánticas Claude.
5. Mantener session-only APIs explícitas donde sean producto (`list`, `insights`).

**Out**:
- Rediseño completo de search ranking.
- Cambio de DB obligatorio.

## Estado inicial esperado

- `src/storage/sqlite.rs` contiene mapeo especial de source y role.
- Algunas queries hardcodean `source = 'session'`.

## Criterios de Aceptación

- `--source ke`, `--source decision`, etc. filtran realmente.
- BM25 e hybrid devuelven resultados consistentes bajo filtros.
- Tests cubren source arbitrario y filtros combinados.
- Session-only queries quedan intencionales y documentadas.

## Fuente de verdad

- `src/storage/sqlite.rs`
- `src/main.rs`
- `tests/cli.rs`
- `tests/lib_api.rs`
