---
ejecutable_en: 1 sesion
estado: Completed # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T084: Configure git-cliff and generate CHANGELOG.md

**Story**: [S054 Changelog & Version](README.md)
**Contribuye a**: CHANGELOG.md generado automaticamente con categorias por tipo de commit

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Backscroll usa conventional commits (feat, fix, refactor, etc.) pero no tiene CHANGELOG. git-cliff es un generador de changelogs que parsea commits convencionales y produce markdown categorizado. Se necesita un `cliff.toml` con la configuracion y ejecutar la generacion inicial.

## Alcance

**In**:
1. Instalar git-cliff como dev dependency o CI tool
2. Crear `cliff.toml` con grupos por tipo (Features, Bug Fixes, Refactor, etc.)
3. Generar CHANGELOG.md inicial desde todos los commits existentes
4. Agregar receta `just changelog` al Justfile

**Out**: Integracion automatica en CI (se hace en T086)

## Estado inicial esperado

- Conventional commits existentes en el historial de git
- Justfile con recetas existentes

## Criterios de Aceptacion

- `cliff.toml` existe con grupos de commit configurados
- `just changelog` genera CHANGELOG.md sin errores
- CHANGELOG.md contiene al menos las secciones Features y Bug Fixes
- Formato markdown es legible y categorizado por tipo

## Fuente de verdad

- `cliff.toml` — new config file
- `Justfile` — add changelog recipe
- `CHANGELOG.md` — generated output
