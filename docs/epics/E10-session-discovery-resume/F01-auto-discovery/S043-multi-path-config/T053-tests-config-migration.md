---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T053: Tests config migration

**Story**: [S043 Multi-path config](README.md)
**Contribuye a**: P2 — config soporta multiples directorios de sesion

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV3: Sync existente preservado
  - Verificar: test de backward compat pasa

## Contexto

Verificar que tanto config legacy (single string) como nueva (array) funcionan.

## Especificacion Tecnica

En `src/config.rs` tests:

1. Test: TOML con `session_dir = "/tmp"` carga como `session_dirs: vec!["/tmp"]`
2. Test: TOML con `session_dirs = ["/a", "/b"]` carga como `session_dirs: vec!["/a", "/b"]`
3. Test: env var `BACKSCROLL_SESSION_DIRS` funciona
4. Test: `default_with_paths()` retorna vec con "."

En `tests/cli.rs`:
5. Integration test: `backscroll sync --path <tempdir>` sigue funcionando

## Alcance

**In**: Unit tests en config.rs, integration test en cli.rs
**Out**: No test de discovery (T056)

## Criterios de Aceptacion

- 4+ unit tests en config.rs
- 1 integration test backward compat
- `just test` pasa

## Fuente de verdad

- `src/config.rs` — mod tests
- `tests/cli.rs`
