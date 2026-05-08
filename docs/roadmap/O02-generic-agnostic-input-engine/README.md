---
estado: Pending
tipo: outcome
---
# O02: Generic agnostic input engine

## Objetivo

Convertir Backscroll en un motor de ingestión genérico donde toda semántica específica de CLI/agente vive en archivos `*.inputs.toml` (`claude.inputs.toml`, `pi.inputs.toml`, etc.) y el core solo interpreta mecanismos genéricos: discovery, decode, selectors, filters, mapping, normalization e indexing.

## Criterios de Éxito

- CE1: Claude y Pi se indexan desde presets TOML, no desde parsers hardcodeados.
  - Verificar: tests de fixtures `claude.inputs.toml` y `pi.inputs.toml` pasan sin llamar parsers `ClaudeInputParser`/`PiInputParser`.
- CE2: `backscroll.toml` queda separado de la configuración de inputs.
  - Verificar: app config no contiene rutas de ingesta; los inputs se cargan desde `*.inputs.toml`.
- CE3: El pipeline canónico es genérico: discover → decode → filter → map → emit.
  - Verificar: `sync`, autosync y `read` pasan por el mismo input engine.
- CE4: Las capacidades específicas (`subagents`, `think`, tags de ruido Claude, mappings Pi) viven en TOML.
  - Verificar: búsqueda textual en `src/` no encuentra esas semánticas como decisiones hardcodeadas del core.

## Invariantes

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: fixtures Claude/Pi producen `ParsedFile.source == "session"`.
- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: `Database::sync_files` no requiere cambios de modelo para inputs JSONL.
- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: no hay ejecución de comandos externos en el input engine.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: `Cargo.toml` no añade `jmespath` salvo decisión posterior explícita.

## Alcance

**In**:
- Loader de `*.inputs.toml`.
- Discovery genérico con globs.
- Parser genérico JSONL con selectores JSONPath.
- Filtros y transforms declarativos mínimos.
- Presets `claude.inputs.toml` y `pi.inputs.toml`.
- Refactor de `sync`, autosync, `read` y tests al input engine.
- Validación/dry-run de inputs.

**Out**:
- Plugins/script adapters ejecutables.
- JMESPath como dependencia del MVP.
- Cambios de esquema SQLite obligatorios.
- Compatibilidad legacy con `session_dirs`, `--path` o fallback Claude si contradice el modelo TOML-only.

## Tasks

| Task | Descripción |
|------|-------------|
| [T001](T001-define-generic-input-contract.md) | Definir contrato TOML genérico para inputs |
| [T002](T002-separate-app-config-from-input-config.md) | Separar app config de input config |
| [T003](T003-implement-glob-discovery.md) | Implementar discovery declarativo con globset |
| [T004](T004-implement-jsonl-jsonpath-engine.md) | Implementar decoder JSONL y selectors JSONPath |
| [T005](T005-implement-declarative-filters-transforms.md) | Implementar filtros/transforms declarativos |
| [T006](T006-create-claude-input-preset.md) | Crear preset `claude.inputs.toml` |
| [T007](T007-create-pi-input-preset.md) | Crear preset `pi.inputs.toml` |
| [T008](T008-refactor-sync-read-api-to-input-engine.md) | Refactorizar sync/read/API al input engine |
| [T009](T009-unify-document-sources-under-inputs.md) | Unificar plans y document sources bajo inputs |
| [T010](T010-add-input-validation-dry-run.md) | Agregar validación y dry-run de inputs |
| [T011](T011-make-search-filters-generic.md) | Hacer filtros downstream genéricos y consistentes |
| [T012](T012-add-manifest-regression-tests.md) | Agregar regresión integral manifest-driven |
| [T013](T013-evaluate-jmespath-future.md) | Evaluar JMESPath como mapping futuro |
| [T014](T014-reconcile-roadmap-with-final-input-contract.md) | Reconciliar tasks O02 con el contrato final de inputs |
