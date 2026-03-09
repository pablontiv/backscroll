---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T054: Discovery function

**Story**: [S044 Directory discovery](README.md)
**Contribuye a**: P1 — sync descubre directorios legacy + actual sin config manual

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Claude Code ha usado multiples layouts de directorio de sesiones. Backscroll debe descubrirlos automaticamente.

## Especificacion Tecnica

En `src/config.rs`:

1. Implementar `pub fn discover_session_dirs() -> Vec<PathBuf>`
2. Probar paths conocidos bajo `$HOME/.claude/`:
   - `~/.claude/projects/*/` (project-scoped sessions)
   - Cualquier directorio que contenga archivos `.jsonl`
3. Filtrar: solo retornar directorios que existen
4. Log descubiertos a `tracing::info`
5. Retornar vec vacio si no se encuentra nada

## Alcance

**In**: Funcion discover_session_dirs() en config.rs
**Out**: No integrar con sync aun (T055), no cambiar Config struct

## Criterios de Aceptacion

- Funcion retorna Vec<PathBuf> con dirs existentes
- Dirs inexistentes omitidos silenciosamente
- Log a nivel info de los dirs encontrados

## Fuente de verdad

- `src/config.rs` — discover_session_dirs()
