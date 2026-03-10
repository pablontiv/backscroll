# F01: Topic Discovery

**Epic**: [E12 Session Analytics](../README.md)
**Objetivo**: Exponer term frequencies del indice FTS5 como subcomando `backscroll topics`, usando la tabla virtual fts5vocab de SQLite.
**Satisface**: P1 (topics retorna terminos rankeados)
**Milestone**: `backscroll topics --robot` retorna los terminos mas frecuentes del corpus con conteos de sesiones y menciones.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E12)
- INV2: `just check` pasa (heredado de E12)
- INV3: Zero dependencias nuevas (heredado de E12)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S050](S050-fts5vocab-schema/) | fts5vocab schema & queries |
| [S051](S051-topics-subcommand/) | Topics subcommand & output |
