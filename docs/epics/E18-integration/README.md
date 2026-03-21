# E18: Integration

**Metrica de exito**: Backscroll es consumible nativamente por Claude Code via MCP y por herramientas externas via export estructurado
**Timeline**: 2026-Q2 — planificado (F02 requiere investigacion)

## Intencion

Backscroll hoy se consume via CLI (directamente o via skill que envuelve llamadas shell). Este epic agrega dos vectores de integracion:

1. **Export**: Resultados de busqueda exportables a Markdown/CSV para consumo en Obsidian, hojas de calculo, o pipelines de datos.
2. **MCP Server**: Exponer search/sync/topics como herramientas MCP (stdio transport), eliminando la indirreccion shell del skill actual.

Contexto competitivo: Multiples herramientas de busqueda de sesiones (Session Finder, Session Search) ya operan como MCP servers. Backscroll tiene ventaja funcional (indexing, BM25, multi-source) pero desventaja de integracion.

## Postcondiciones

- P1: `backscroll export --format markdown "query"` produce Markdown con resultados de busqueda
- P2: `backscroll export --format csv "query"` produce CSV con headers
- P3: MCP server expone `backscroll_search`, `backscroll_sync`, `backscroll_topics` como tools (si F02 se implementa)

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa
- INV3: Export reutiliza SearchEngine trait y output module existentes
- INV4: MCP server es feature-flag (`--features mcp`) — no afecta binary size default

## Out of Scope

- Integracion bidireccional (backscroll no escribe en Obsidian/Notion)
- Export de sesiones completas (solo resultados de busqueda)
- HTTP server (solo MCP stdio transport)
- Cross-machine sync / replicacion de DB

## Features

| ID | Nombre | Descripcion |
|----|--------|-------------|
| F01 | [Export Formats](F01-export-formats/) | Exportar resultados de busqueda a Markdown y CSV |
| F02 | [MCP Server](F02-mcp-server/) | Modo servidor MCP (stdio) para integracion nativa con Claude Code |

## Orden de Ejecucion

| Feature | Depende de | Razon |
|---------|-----------|-------|
| F01 | — | Export es extension directa del output module, bajo riesgo |
| F02 | — | MCP es independiente pero requiere investigacion de SDK Rust |

## Decision Log

| Fecha | Decision | Razon |
|-------|----------|-------|
| 2026-03-20 | MCP como feature-flag separado | Evita agregar deps de MCP al binario principal, mantiene build lean |
| 2026-03-20 | Solo stdio transport, no HTTP | Consistente con patron Claude Code MCP servers, menos superficie de ataque |

## Gaps Activos

- Evaluar madurez de Rust MCP SDK (rmcp, mcp-rs) antes de comprometer F02
- Decidir si export va como subcomando o flag de search (`search --export md`)
