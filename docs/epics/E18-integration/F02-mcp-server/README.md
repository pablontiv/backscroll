# F02: MCP Server

**Epic**: [E18 Integration](../README.md)
**Objetivo**: Exponer backscroll como servidor MCP (Model Context Protocol) para integracion nativa con Claude Code
**Satisface**: P3 (MCP tools backscroll_search, backscroll_sync, backscroll_topics)
**Milestone**: `backscroll serve` inicia servidor MCP stdio consumible por Claude Code

## Investigacion Requerida

Este feature requiere evaluacion de madurez del ecosistema Rust MCP:
- **rmcp** / **mcp-rs**: Evaluar estabilidad, API surface, mantenimiento
- **Alternativa**: JSON-RPC manual sobre stdio (mas control, menos deps)
- **Decision gate**: Prototipar con SDK antes de comprometer implementacion completa

## Invariantes

- INV1: Feature-flag `--features mcp` — no afecta binary size default
- INV2: Mismo SearchEngine trait usado por CLI — zero duplicacion de logica
- INV3: Solo stdio transport (no HTTP, no WebSocket)

## Stories

| Story | Descripcion |
|-------|-------------|
| S085 | Investigacion: evaluar Rust MCP SDKs (rmcp, mcp-rs) |
| S086 | Tool definitions: backscroll_search, backscroll_sync, backscroll_topics |
| S087 | Servidor stdio: loop de lectura JSON-RPC, dispatch a SearchEngine |
| S088 | Subcomando `serve` con feature-flag |
| S089 | Tests: integracion MCP con mock client |
