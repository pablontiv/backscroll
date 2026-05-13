---
estado: Specified
tipo: task
---
# T006: Project identity y reader

**Outcome**: [Port a Go](README.md)

## Contexto

Dos módulos que dependen de storage: el registry de identidad de proyectos (resolve canonical project IDs con niveles de confianza) y el reader de archivos de sesión individuales. Equivalentes a `core/projects.rs` y `core/reader.rs`.

## Alcance

**In**:
1. `internal/projects` — `LoadGlobalRegistry()` lee `~/.config/backscroll/projects.toml`, `LoadLocalHint()` sube directorios buscando `.backscroll/project.toml`, `Identify()` resuelve IDs con niveles exact/pattern/hint/truncated/unknown.
2. `internal/reader` — lectura directa y filtrado de archivos de sesión individuales; usado por el comando `read`.
3. Tests con los fixtures y el registry de proyectos existente.

**Out**:
1. Comandos CLI (van en T007).

## Criterios de Aceptación

- `Identify()` resuelve correctamente proyectos conocidos del registry global existente.
- `LoadLocalHint()` sube hasta 4 niveles sin entrar en pánico si `.backscroll/project.toml` no existe.
- `go test ./internal/projects/... ./internal/reader/...` pasa.
