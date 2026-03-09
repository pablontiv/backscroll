---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T051: Extend Config struct para multi-path

**Story**: [S043 Multi-path config](README.md)
**Contribuye a**: P2 — config soporta multiples directorios de sesion

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Sync existente preservado con `--path` explicito
  - Verificar: tests existentes pasan

## Contexto

`Config` actualmente tiene `session_dir: String` (un solo path). Para auto-discovery necesitamos soportar multiples paths.

## Especificacion Tecnica

En `src/config.rs`:

1. Cambiar `session_dir: String` a `session_dirs: Vec<String>`
2. Agregar `#[serde(alias = "session_dir")]` para backward compat con TOML legacy
3. Implementar deserializador custom que acepta tanto string como array (serde `string_or_vec`)
4. Actualizar `default_with_paths()` para retornar `vec![".".into()]`

## Alcance

**In**: Cambiar struct Config, custom deserializer, default_with_paths()
**Out**: No cambiar CLI args (T052), no cambiar sync dispatch (T055)

## Criterios de Aceptacion

- `Config.session_dirs` es `Vec<String>`
- TOML con `session_dir = "/a"` deserializa como `vec!["/a"]`
- TOML con `session_dirs = ["/a", "/b"]` deserializa como `vec!["/a", "/b"]`
- `default_with_paths()` retorna vec con "."

## Fuente de verdad

- `src/config.rs` — struct Config
