# S043: Multi-path config

**Feature**: [F01 Auto-Discovery de Directorios](../README.md)
**Capacidad**: Config acepta tanto `session_dir` (string, legacy) como `session_dirs` (array) con backward compatibility. CLI `--path` acepta multiples valores.
**Cubre**: P2 del Epic (config multi-path)

## Antes / Despues

**Antes**: `Config.session_dir: String` contiene un solo path. `--path` acepta un valor.

**Despues**: `Config.session_dirs: Vec<String>` contiene multiples paths. `session_dir` en TOML sigue funcionando (deserializado como vec de un elemento). `--path` acepta multiples valores via `Vec<String>`.

## Criterios de Aceptacion (semanticos)

- [ ] Config con `session_dirs = ["/path/a", "/path/b"]` carga correctamente
- [ ] Config legacy con `session_dir = "/path/a"` sigue cargando (backward compatible)
- [ ] `--path /a --path /b` funciona en CLI

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Sync existente preservado con `--path` explicito
  - Verificar: `backscroll sync --path /tmp/test` sigue funcionando

## Tasks

| Task | Descripcion |
|------|-------------|
| [T051](T051-extend-config-struct.md) | Extend Config struct para multi-path |
| [T052](T052-multi-path-cli-flag.md) | Multi-path CLI flag |
| [T053](T053-tests-config-migration.md) | Tests config migration |

## Fuente de verdad

- `src/config.rs` — struct Config y load()
- `src/main.rs` — CLI arg parsing y sync dispatch
