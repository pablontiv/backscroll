---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T055: Integration into sync

**Story**: [S044 Directory discovery](README.md)
**Contribuye a**: P1 — sync descubre directorios legacy + actual sin config manual

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Sync existente preservado con --path explicito
  - Verificar: `backscroll sync --path /tmp` sigue funcionando

## Contexto

Conectar discover_session_dirs() con el sync dispatch en main.rs.

## Especificacion Tecnica

En `src/main.rs` sync dispatch:

1. Si `--path` no esta vacio → usar esos paths (override)
2. Si `config.session_dirs` no es default → usar config
3. Else → llamar `discover_session_dirs()` y usar resultado
4. Si no hay paths → error informativo al usuario
5. Iterar sobre paths, llamar `parse_sessions()` por cada uno, concatenar resultados

## Alcance

**In**: Logica de resolucion de paths en sync dispatch
**Out**: No cambiar parse_sessions() ni SearchEngine

## Criterios de Aceptacion

- Sync sin args usa discovery si no hay config explicita
- Sync con --path ignora discovery
- Sin paths descubiertos produce error claro

## Fuente de verdad

- `src/main.rs` — Commands::Sync dispatch
