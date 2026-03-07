---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T002: Actualizar sync.rs para parsear SessionRecord

**Story**: [S021 Reescribir modelo para JSONL real](README.md)
**Contribuye a**: Parser procesa JSONL real de Claude Code sin errores

[[blocks:T001-session-record-wrapper]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

`sync.rs` actualmente deserializa cada linea JSONL como `ClaudeMessage`. Debe cambiar a `SessionRecord` y extraer `record.message.role` y `record.message.content` para indexar. Los metadatos del wrapper (uuid, timestamp) se pasan downstream.

## Alcance

**In**:
1. Cambiar `serde_json::from_str::<ClaudeMessage>` a `serde_json::from_str::<SessionRecord>` en sync.rs
2. Extraer `record.message.role` y text content del message
3. Pasar uuid y timestamp a la llamada de index_message (o struct intermedio)
4. Manejar gracefully records que no tengan campo `message` (skip con warning)

**Out**: No modificar schema SQLite (eso es S022). No modificar test fixtures (eso es T003).

## Estado inicial esperado

- `SessionRecord` struct definido en models.rs (T001 completado)
- sync.rs usa `ClaudeMessage` directamente

## Criterios de Aceptacion

- `grep "SessionRecord" src/core/sync.rs` encuentra uso
- `cargo check` compila sin errores
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs`
- `src/core/models.rs`
