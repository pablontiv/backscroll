# S021: Reescribir modelo para JSONL real

**Feature**: [F01 Parser y Schema](../README.md)
**Capacidad**: El parser procesa correctamente records JSONL reales de Claude Code con formato wrapper.
**Cubre**: P1 del Epic (parser procesa JSONL real)

## Antes / Despues

**Antes**: El parser (`ClaudeMessage`) espera `{role, content}` en el top-level del JSON, pero los records reales de Claude Code tienen un wrapper `{type, message: {role, content}, uuid, timestamp, sessionId, slug, ...}`. Esto causa que `sync` falle silenciosamente en datos reales — zero mensajes se indexan.

**Despues**: `SessionRecord` parsea el formato wrapper real. El campo `message` se extrae correctamente, y los metadatos (uuid, timestamp, sessionId, slug) se preservan para uso downstream. Los test fixtures usan el formato real.

## Criterios de Aceptacion (semanticos)

- [ ] Parser procesa JSONL real de Claude Code sin errores
- [ ] Metadatos del wrapper (uuid, timestamp, slug) son accesibles post-parse

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T001](T001-session-record-wrapper.md) | Definir SessionRecord wrapper struct |
| [T002](T002-sync-parsear-wrapper.md) | Actualizar sync.rs para parsear SessionRecord |
| [T003](T003-fixtures-formato-real.md) | Actualizar test fixtures al formato wrapper |

## Fuente de verdad

- `src/core/models.rs` — structs de dominio
- `src/core/sync.rs` — logica de parsing
- `tests/cli.rs` — integration tests con fixtures
