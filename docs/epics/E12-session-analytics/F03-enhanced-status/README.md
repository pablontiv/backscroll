# F03: Enhanced Status

**Epic**: [E12 Session Analytics](../README.md)
**Objetivo**: Mejorar `backscroll status` para incluir desglose de sesiones y mensajes por proyecto.
**Satisface**: P3 (status incluye breakdown por proyecto)
**Milestone**: `backscroll status` muestra seccion adicional con tabla project/sessions/messages.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E12)
- INV2: `just check` pasa (heredado de E12)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S053](S053-project-breakdown/) | Per-project breakdown |
