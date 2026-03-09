---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T065: Plan sync in main.rs

**Story**: [S048 Plan sync pipeline](README.md)
**Contribuye a**: P1 — plans indexados

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV4: Sync incremental funciona para plans
  - Verificar: segundo sync no re-procesa plans sin cambios

## Contexto

Agregar discovery y parsing de plans al comando sync.

## Especificacion Tecnica

En `src/main.rs` sync dispatch:

1. Despues de sync de sessions, descubrir `~/.claude/plans/`
2. Encontrar todos los archivos `*.md` en ese directorio
3. Para cada plan: computar hash, verificar contra hashes existentes, parsear si nuevo/cambiado
4. Llamar `engine.sync_files(plan_files)`
5. Agregar flag `--no-plans` para omitir plan sync (default: incluir plans)
6. Log: "Sincronizando N plans desde ~/.claude/plans/"

## Alcance

**In**: Plan discovery, parsing, sync en main.rs
**Out**: No agregar --source filter (T067)

## Criterios de Aceptacion

- `backscroll sync` indexa sessions Y plans
- `backscroll sync --no-plans` omite plans
- Plans sin cambios se omiten en re-sync

## Fuente de verdad

- `src/main.rs` — Commands::Sync dispatch
