---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T082: Update status output format

**Story**: [S053 Per-project breakdown](README.md)
**Contribuye a**: P3 (status incluye breakdown por proyecto)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Update status output to include per-project breakdown section.

## Especificacion Tecnica

After existing status output, add "By Project:" section with aligned table showing project, sessions, messages.

## Alcance

**In**: After existing status output, add "By Project:" section with aligned table showing project, sessions, messages.
**Out**: No query function (T081), no tests (T083)

## Criterios de Aceptacion

- `backscroll status` shows new section
- Existing stats unchanged
- `just check` pasa

## Fuente de verdad

- `src/main.rs` (status command handler)
