# F02: Limpieza de Error Handling

**Epic**: [E09 Hardening Post-Validacion](../README.md)
**Objetivo**: Eliminar dead code en `errors.rs` y verificar coherencia del error handling.
**Satisface**: P2 (zero dead code)
**Milestone**: `just check` pasa sin `#[allow(dead_code)]` en el codebase.

## Invariantes

- INV2: `just check` pasa (heredado de E09)
- INV3: Tests existentes no regresan (heredado de E09)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S042](S042-resolver-dead-code/) | Resolver dead code en errors.rs |
