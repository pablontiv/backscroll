# F01: Auto-Discovery de Directorios de Sesion

**Epic**: [E10 Session Discovery & Smart Resume](../README.md)
**Objetivo**: Detectar y recorrer todos los layouts de directorios de sesion conocidos de Claude Code. Soportar config multi-path.
**Satisface**: P1 (auto-discovery), P2 (multi-path config)
**Milestone**: `backscroll sync` sin argumentos indexa sesiones de ambos layouts de directorio.

## Invariantes

- INV1: `cargo test --all-features` pasa (heredado de E10)
- INV2: `just check` pasa (heredado de E10)
- INV3: Sync existente preservado con `--path` explicito (heredado de E10)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S043](S043-multi-path-config/) | Multi-path config |
| [S044](S044-directory-discovery/) | Directory discovery |
