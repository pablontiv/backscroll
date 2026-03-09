---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T057: Add Resume to Commands enum

**Story**: [S045 Resume subcommand](README.md)
**Contribuye a**: P3 — resume produce session ID usable por claude --resume

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Agregar `Resume` como quinto subcommand CLI.

## Especificacion Tecnica

En `src/main.rs`:

1. Agregar a Commands enum:
   ```rust
   Resume {
       query: String,
       #[arg(short, long)]
       project: Option<String>,
       #[arg(long, default_value_t = false)]
       robot: bool,
   }
   ```
2. Agregar match arm basico en dispatch (placeholder que llama search con limit 1)

## Alcance

**In**: Variant en enum, match arm basico
**Out**: Logica completa de resume (T058), session ID resolution (T060)

## Criterios de Aceptacion

- `backscroll resume --help` muestra ayuda
- `backscroll resume "test"` no paniquea (aunque no tenga logica completa)

## Fuente de verdad

- `src/main.rs` — Commands enum
