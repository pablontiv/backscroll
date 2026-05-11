---
estado: Completed
tipo: outcome
---
# O01: Refactor de parser de sesiones para inputs declarativos

## Objetivo

Migrar Backscroll a una capa de inputs declarativa para sesiones y fuentes de datos, permitiendo agregar nuevos adapters vía manifiestos TOML sin recompilar.

## Criterios de Éxito

- CE1: Backscroll carga y procesa entradas definidas en `backscroll.inputs.toml` y `backscroll.inputs.d/*.toml`.
  - Verificar: `backscroll sync --path <dir>` indexa por `source="session"` sin cambios en configuración legacy.
- CE2: El parser existente de sesiones de Claude se mantiene operando con comportamiento equivalente.
  - Verificar: suite de tests existente y de regresión de precedencia pasan.

## Invariantes

- INV1: Mantener `source="session"` para entradas de sesiones.
  - Verificar: los índices nuevos preservan `source` igual que hoy.
- INV2: Mantener orden de precedencia `--path` > configuración > inputs > fallback.
  - Verificar: pruebas unitarias explícitas.

## Alcance

**In**:
- Capa declarativa de inputs para sesiones.
- Parser nativo de `claude` y `pi`.
- Compatibilidad de parser y pruebas de regresión.

**Out**:
- Cambios de esquema SQLite.
- Adapters ejecutables externos (`command` adapters).

## Tasks

| Task | Descripción |
|------|-------------|
| [T001](T001-define-input-manifest-contract.md) | Definir contrato TOML para inputs externos |
| [T002](T002-implement-configuration-loading-for-inputs.md) | Integrar carga y normalización de inputs en Config |
| [T003](T003-extract-claude-parser-as-native-input.md) | Extraer parser existente en adapter nativo `claude` |
| [T004](T004-design-session-input-parser-interface.md) | Diseñar trait/registry de `SessionInputParser` |
| [T005](T005-add-declarative-session-input-parser.md) | Implementar parser `source="session"` desde manifiestos |
| [T006](T006-add-pi-input-support.md) | Añadir soporte inicial de input `pi` |
| [T007](T007-implement-session-path-precedence.md) | Ajustar resolución de paths con precedencia |
| [T008](T008-preserve-ingestion-compatibility.md) | Preservar semántica de ingestión existente |
| [T009](T009-add-precedence-regression-tests.md) | Añadir tests de precedencia y compatibilidad |
| [T010](T010-update-documentation-for-input-configuration.md) | Actualizar documentación y ejemplos |
| [T011](T011-run-validation-and-fix-regressions.md) | Ejecutar checks y cerrar cambios de implementación |
