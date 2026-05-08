---
estado: Completed
tipo: task
---
# T009: Unify plans and external document sources under inputs

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE3

[[blocked_by:./T008-refactor-sync-read-api-to-input-engine.md]]

## Preserva

- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: markdown/document inputs también emiten esas estructuras.

## Contexto

Además de sesiones, Backscroll tiene paths hardcodeados para plans y source registries (`ke`, `decision`, `memory`, `rule`, `spec`, `backlog`). Para agnosticismo completo deben declararse como inputs.

## Alcance

**In**:
1. Diseñar/implementar `decode.format = "markdown"` y/o `markdown_sections`.
2. Reemplazar `~/.claude/plans` por un input declarativo `source = "plan"`.
3. Reemplazar `SourcesConfig`/`SourceRegistry` por inputs declarativos para `ke`, `decision`, `memory`, `rule`, `spec`, `backlog`.
4. Soportar split por headers para casos `plan`/`spec`.
5. Actualizar docs y tests de sources.

**Out**:
- Cambios de esquema SQLite.
- Reescribir search ranking.

## Estado inicial esperado

- `src/core/plans.rs` hardcodea `source = "plan"`.
- `src/core/sources.rs` hardcodea source types y parsers markdown.
- `src/main.rs` descubre `~/.claude/plans`.

## Criterios de Aceptación

- Plans se sincronizan desde un input TOML, no desde path Claude hardcodeado.
- External sources se sincronizan desde inputs TOML, no desde `[sources]` app config.
- Tests existentes de plan/source pasan adaptados al engine.
- `source` sigue reflejando `plan`, `ke`, `decision`, etc. en DB.

## Fuente de verdad

- `src/core/plans.rs`
- `src/core/sources.rs`
- `src/main.rs`
- `docs/configuration.md`
- `docs/sync.md`
