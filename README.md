# Backscroll

**Tier 2 Search for Claude Code Sessions** (Rust Architecture 2026)

Backscroll es un motor de búsqueda de alto rendimiento diseñado para indexar y buscar en el historial de sesiones de Claude Code. Construido en Rust con SQLite FTS5, ofrece una alternativa rápida, segura y estática a la búsqueda tradicional.

## 🚀 Características (Marzo 2026)

- **Motor FTS5 + BM25:** Búsqueda por relevancia nativa en SQLite.
- **Ingesta Defensiva:** Parseo robusto de esquemas mutantes de Claude con `serde(untagged)`.
- **Sincronización Incremental:** Deduplicación basada en hashes SHA-256 para evitar re-indexación.
- **Zero Deps:** Binarios estáticos generados con `cargo-zigbuild`.
- **Calidad Extrema:** >95% de cobertura de código verificado con LLVM.
- **Diagnósticos Modernos:** Errores visuales y amigables con `miette`.

## 🛠 Instalación y Uso

### Configuración
Crea un archivo `backscroll.toml` o usa variables de entorno:
```toml
database_path = "~/.backscroll.db"
session_dir = "~/.claude/sessions"
```

### Comandos Principales
```bash
# Sincronizar sesiones nuevas
backscroll sync

# Buscar en el historial
backscroll search "mejoras sistema tipos"

# Ver estado del índice
backscroll status
```

## 🏗 Desarrollo

Este proyecto utiliza `just` para la automatización:
- `just check`: Ejecuta lints y clippy (rigor nursery).
- `just test`: Ejecuta la suite de pruebas.
- `just coverage-summary`: Genera reporte de cobertura (Objetivo: 85% min).
- `just static-build`: Compila binario estático para Linux musl.

## 📈 Roadmap Administrativo
El progreso detallado se encuentra en `docs/epics/`, gestionado mediante `rootline`.
