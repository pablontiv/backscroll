---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T090: Create install.sh with platform detection and download

**Story**: [S056 Install Script](README.md)
**Contribuye a**: install.sh detecta Linux/macOS y descarga el binario correcto

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`

## Contexto

Un install script estandar para CLIs: detecta OS (Linux/macOS) y arch (x86_64/aarch64), descarga el binario correspondiente del ultimo GitHub Release via API, y lo instala en un directorio del PATH del usuario.

## Alcance

**In**:
1. Crear `install.sh` en la raiz del repo
2. Detectar OS (uname -s) y arch (uname -m)
3. Construir URL del asset del GitHub Release mas reciente
4. Descargar con curl, hacer chmod +x, mover a ~/.local/bin/ o /usr/local/bin/
5. Verificar instalacion con `backscroll --version`

**Out**: Homebrew formula, package managers

## Estado inicial esperado

- GitHub Releases con binarios nombrados backscroll-{os}-{arch}

## Criterios de Aceptacion

- `bash install.sh` en Linux x86_64 instala backscroll correctamente
- `bash install.sh` en macOS aarch64 instala backscroll correctamente
- Script detecta y reporta plataformas no soportadas (ej: Windows, arm32)
- Script es idempotente (re-ejecutar actualiza sin error)

## Fuente de verdad

- `install.sh` — new file
