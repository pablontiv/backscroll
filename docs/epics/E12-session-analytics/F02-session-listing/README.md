# F02: Session Listing

**Epic**: [E12 Session Analytics](../README.md)
**Objetivo**: Agregar subcomando `backscroll list` que lista sesiones indexadas con metadata basica (path, proyecto, timestamps, message count).
**Satisface**: P2 (list retorna sesiones con metadata)
**Milestone**: `backscroll list --recent 5 --robot` retorna las 5 sesiones mas recientes con metadata.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E12)
- INV2: `just check` pasa (heredado de E12)
- INV4: Output consistente con search/resume (heredado de E12)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S052](S052-list-subcommand/) | List subcommand |
