---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T067: Add --source flag to Search

**Story**: [S049 Source filter in search](README.md)
**Contribuye a**: P3 — --source plans filtra solo plans

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Search necesita filtrar por tipo de fuente (sessions, plans, all).

## Especificacion Tecnica

En `src/main.rs`:

1. Agregar a Commands::Search:
   ```rust
   #[arg(long, default_value = "all")]
   source: String, // "sessions" | "plans" | "all"
   ```
2. Pasar `source` al llamado de `engine.search()`

## Alcance

**In**: Arg en enum, pasar a search
**Out**: No cambiar search() signature (T068)

## Criterios de Aceptacion

- `backscroll search "test" --source plans` parsea correctamente
- Default es "all"

## Fuente de verdad

- `src/main.rs` — Commands::Search
