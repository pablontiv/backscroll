# F02: Smart Resume

**Epic**: [E10 Session Discovery & Smart Resume](../README.md)
**Objetivo**: Nuevo subcomando `resume` que busca la sesion mas relevante y produce el session file path / session ID para piping a `claude --resume`.
**Satisface**: P3 (resume produce session ID), P4 (pipe-friendly)
**Milestone**: `backscroll resume "auth refactor" --robot | xargs claude --resume` reanuda la sesion correcta.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E10)
- INV2: `just check` pasa (heredado de E10)
- INV4: Performance < 1s para resume queries (heredado de E10)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S045](S045-resume-subcommand/) | Resume subcommand |
| [S046](S046-session-id-resolution/) | Session ID resolution |
