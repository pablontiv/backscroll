---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T001: Definir SessionRecord wrapper struct

**Story**: [S021 Reescribir modelo para JSONL real](README.md)
**Contribuye a**: Parser procesa JSONL real de Claude Code sin errores

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Los records JSONL reales de Claude Code tienen un formato wrapper: `{type, message: {role, content}, uuid, timestamp, sessionId, slug, ...}`. El parser actual (`ClaudeMessage`) espera `{role, content}` en el top-level y falla silenciosamente en datos reales. Se necesita un nuevo struct `SessionRecord` que refleje el formato real.

## Especificacion Tecnica

```yaml
archivo: src/core/models.rs
struct: SessionRecord
campos:
  - type: String (rename = "type" por ser keyword)
  - message: ClaudeMessage (nested)
  - uuid: Option<String>
  - timestamp: Option<String>
  - sessionId: Option<String> (rename camelCase)
  - slug: Option<String>
  - isMeta: Option<bool> (default false)
serde: deny_unknown_fields NO (schema puede evolucionar)
```

## Alcance

**In**:
1. Definir `SessionRecord` struct con serde derive en `src/core/models.rs`
2. Mantener `ClaudeMessage` existente como struct anidado (field `message`)
3. Agregar `#[serde(rename = "type")]` para campo `record_type`
4. Campos opcionales para metadatos (uuid, timestamp, sessionId, slug, isMeta)

**Out**: No modificar sync.rs (eso es T002). No modificar tests (eso es T003).

## Estado inicial esperado

- `src/core/models.rs` tiene `ClaudeMessage` con `{role, content, is_meta}`
- No existe `SessionRecord`

## Criterios de Aceptacion

- `grep "pub struct SessionRecord" src/core/models.rs` encuentra el struct
- `cargo check` compila sin errores
- `just check` pasa (fmt + clippy)

## Fuente de verdad

- `src/core/models.rs`
