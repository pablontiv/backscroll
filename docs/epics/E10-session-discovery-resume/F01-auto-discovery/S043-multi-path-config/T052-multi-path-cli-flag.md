---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T052: Multi-path CLI flag

**Story**: [S043 Multi-path config](README.md)
**Contribuye a**: P2 — config soporta multiples directorios de sesion

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

CLI `--path` actualmente es `Option<String>`. Necesita aceptar multiples valores.

## Especificacion Tecnica

En `src/main.rs`:

1. Cambiar `Sync { path: Option<String> }` a `Sync { path: Vec<String> }`
2. Usar `#[arg(short, long)]` con Vec para que clap acepte `--path /a --path /b`
3. En dispatch: si `path` no esta vacio, usar los paths dados; si esta vacio, usar `config.session_dirs`
4. Iterar sobre los paths y llamar `parse_sessions()` para cada uno, coleccionando resultados

## Alcance

**In**: Cambiar tipo de arg en Commands::Sync, actualizar dispatch de sync
**Out**: No cambiar parse_sessions() ni SearchEngine

## Criterios de Aceptacion

- `backscroll sync --path /a --path /b` funciona
- `backscroll sync --path /a` sigue funcionando (single path)
- `backscroll sync` sin --path usa config.session_dirs

## Fuente de verdad

- `src/main.rs` — Commands::Sync y match dispatch
