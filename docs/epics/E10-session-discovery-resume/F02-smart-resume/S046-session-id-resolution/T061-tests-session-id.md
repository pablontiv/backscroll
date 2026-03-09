---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T061: Tests session ID resolution

**Story**: [S046 Session ID resolution](README.md)
**Contribuye a**: P3 — resume produce session ID

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Verificar que get_session_id funciona correctamente.

## Especificacion Tecnica

En `src/storage/sqlite.rs` tests:

1. Test: sync file con UUID conocido, query get_session_id, verificar match
2. Test: file sin UUID (msg.uuid = None), verificar fallback a file stem
3. Test: source_path inexistente retorna None

## Alcance

**In**: Unit tests para get_session_id
**Out**: No test de integracion (cubierto por T059)

## Criterios de Aceptacion

- 3 unit tests
- `just test` pasa

## Fuente de verdad

- `src/storage/sqlite.rs` — mod tests
