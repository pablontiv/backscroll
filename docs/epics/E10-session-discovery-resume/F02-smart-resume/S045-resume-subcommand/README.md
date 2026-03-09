# S045: Resume subcommand

**Feature**: [F02 Smart Resume](../README.md)
**Capacidad**: `backscroll resume <query>` realiza una busqueda, toma el top result, y produce su session path y session ID.
**Cubre**: P3 (resume produce session ID), P4 (pipe-friendly)

[[blocks:S043-multi-path-config]]

## Antes / Despues

**Antes**: Usuario debe `backscroll search`, inspeccionar resultados manualmente, extraer path, buscar UUID, y luego `claude --resume <id>`.

**Despues**: `backscroll resume "refactor"` produce `SESSION_ID\tPATH` (robot mode) o texto formateado. Soporta filtro `--project`.

## Criterios de Aceptacion (semanticos)

- [ ] Resume retorna solo el top-1 resultado
- [ ] Text mode muestra session path + session ID + preview del primer mensaje de usuario
- [ ] Robot mode produce `session_id\tpath` (single line, pipe-ready)
- [ ] `--project` filter funciona
- [ ] Resultados vacios producen mensaje informativo

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV4: Performance < 1s
  - Verificar: `time backscroll resume "test"` < 1s

## Tasks

| Task | Descripcion |
|------|-------------|
| [T057](T057-add-resume-to-commands.md) | Add Resume to Commands enum |
| [T058](T058-resume-search-logic.md) | Resume search logic |
| [T059](T059-resume-integration-test.md) | Resume integration test |

## Fuente de verdad

- `src/main.rs` — Commands enum y dispatch
- `src/output.rs` — format_results()
