# S044: Directory discovery

**Feature**: [F01 Auto-Discovery de Directorios](../README.md)
**Capacidad**: Cuando no hay paths explicitos configurados, backscroll descubre todos los directorios de sesion conocidos de Claude Code bajo `~/.claude/`.
**Cubre**: P1 del Epic (auto-discovery)

## Antes / Despues

**Antes**: Default session_dir es `.` (directorio actual). Usuario debe configurar o pasar `--path`.

**Despues**: Default descubre `~/.claude/projects/*/`, incluyendo layouts legacy (`local-agent-mode-sessions/`) y actual (`claude-code-sessions/`). Falls back a paths configurados si ninguno descubierto.

## Criterios de Aceptacion (semanticos)

- [ ] Discovery encuentra ambos layouts de directorio (legacy y actual)
- [ ] Directorios inexistentes se omiten silenciosamente
- [ ] Resultados de discovery se loguean a nivel `tracing::info`
- [ ] Config explicita overridea auto-discovery

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T054](T054-discovery-function.md) | Discovery function |
| [T055](T055-integration-into-sync.md) | Integration into sync |
| [T056](T056-tests-discovery.md) | Tests discovery |

## Fuente de verdad

- `src/config.rs` — discover_session_dirs()
- `src/main.rs` — sync dispatch
