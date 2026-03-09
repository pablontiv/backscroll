# E10: Session Discovery & Smart Resume

**Objetivo**: Auto-descubrir todos los directorios de sesion conocidos de Claude Code y proveer un comando `resume` que encuentra la sesion mas relevante por contenido y produce su session ID para piping a `claude --resume`.

## Postcondiciones

| # | Postcondicion | Features | Verificacion |
|---|---------------|----------|-------------|
| P1 | Sync descubre directorios legacy y actual sin config manual | F01 | `backscroll sync` sin `--path` indexa de ambos dirs si existen |
| P2 | Config soporta multiples directorios de sesion | F01 | `session_dirs = ["/a", "/b"]` en TOML funciona |
| P3 | `backscroll resume <query>` produce session ID usable por `claude --resume` | F02 | `backscroll resume "refactor" --robot` produce single line con path |
| P4 | Resume en `--robot` es pipe-friendly (single line, session-id only) | F02 | Output es una linea tab-separated |

## Invariantes

- INV1: `cargo test --all-features` pasa
- INV2: `just check` pasa (clippy nursery+pedantic, -D warnings)
- INV3: Sync existente preservado para usuarios con `--path` explicito o config
- INV4: Performance < 1s para resume queries sobre corpus indexado

## Out of Scope

- Plan indexing (E11)
- MCP server mode (descartado — CLI+skill mas eficiente en tokens)
- Deprecar sessions-index.json (valor de enrichment preservado)
- Investigacion de valor de subagent sessions (S2)

## Features

| Feature | Descripcion |
|---------|-------------|
| [F01](F01-auto-discovery/) | Auto-Discovery de Directorios de Sesion |
| [F02](F02-smart-resume/) | Smart Resume |

## Dependencias

Ninguna. Este es el primer epic del roadmap v2.
