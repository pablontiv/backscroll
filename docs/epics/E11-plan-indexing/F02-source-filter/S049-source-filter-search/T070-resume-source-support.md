---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T070: Resume source support

**Story**: [S049 Source filter in search](README.md)
**Contribuye a**: P3 — source filter en resume

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Resume command tambien debe soportar --source para buscar en plans o sessions especificamente.

## Especificacion Tecnica

En `src/main.rs`:

1. Agregar a Commands::Resume:
   ```rust
   #[arg(long, default_value = "all")]
   source: String,
   ```
2. Pasar source al search call en resume dispatch

## Alcance

**In**: Arg en resume, pasar a search
**Out**: No test dedicado (cubierto por tests existentes de resume + source)

## Criterios de Aceptacion

- `backscroll resume "test" --source sessions` funciona
- Default es "all"

## Fuente de verdad

- `src/main.rs` — Commands::Resume
