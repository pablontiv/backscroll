---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T003: Actualizar test fixtures al formato wrapper

**Story**: [S021 Reescribir modelo para JSONL real](README.md)
**Contribuye a**: Metadatos del wrapper son accesibles post-parse

[[blocks:T002-sync-parsear-wrapper]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Los test fixtures actuales en `tests/cli.rs` usan formato simplificado `{role: "user", content: "text"}` que no match el formato real. Deben actualizarse al formato wrapper `{type: "user", message: {role: "user", content: "text"}, uuid: "...", timestamp: "..."}`.

## Alcance

**In**:
1. Actualizar fixtures en `tests/cli.rs` al formato wrapper
2. Agregar test `test_parse_real_jsonl` que parsee una fixture representativa
3. Incluir records de distintos tipos (user, assistant, progress) para verificar que solo user/assistant se procesan
4. Verificar que uuid y timestamp se extraen correctamente

**Out**: No agregar filtrado de ruido (eso es E07). Solo verificar parsing basico.

## Estado inicial esperado

- sync.rs parsea `SessionRecord` (T002 completado)
- Fixtures usan formato simplificado

## Criterios de Aceptacion

- `cargo test test_parse_real_jsonl` pasa
- Fixtures en tests/ usan formato wrapper `{type, message: {...}, uuid, timestamp}`
- `cargo test` — todos los tests pasan

## Fuente de verdad

- `tests/cli.rs`
