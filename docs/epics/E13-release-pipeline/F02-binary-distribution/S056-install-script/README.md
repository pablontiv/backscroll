# S056: Install Script

**Feature**: [F02 Binary Distribution](../README.md)
**Capacidad**: Usuarios instalan backscroll con un solo comando curl
**Cubre**: P3 (instalacion sin Rust toolchain)

## Antes / Despues

**Antes**: Instalacion requiere `cargo install backscroll` con Rust toolchain completo. No hay install script ni instrucciones para instalacion directa de binario.

**Despues**: `curl -fsSL .../install.sh | bash` detecta la plataforma, descarga el binario correcto desde GitHub Releases, y lo instala en `~/.local/bin/` o `/usr/local/bin/`. README documenta ambos metodos de instalacion.

## Criterios de Aceptacion (semanticos)

- [ ] install.sh detecta Linux/macOS y x86_64/aarch64
- [ ] install.sh descarga el binario correcto del ultimo GitHub Release
- [ ] README tiene secciones claras de instalacion (binario, cargo, source)

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T090](T090-install-sh.md) | Create install.sh with platform detection and download |
| [T091](T091-readme-install-instructions.md) | Update README with installation instructions |

## Fuente de verdad

- `install.sh` — new file
- `README.md` — installation section
