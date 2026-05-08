---
estado: Specified
tipo: task
---
# T014: Reconcile O02 roadmap with final input contract

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE2, CE3, CE4

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: las tasks actualizadas no piden `source = "claude"` ni `source = "pi"` para conversaciones.
- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: las tasks actualizadas no agregan `command`, `exec`, `script` ni adapters ejecutables.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: las tasks actualizadas dejan JMESPath en T013 y usan JSONPath/selectores MVP.

## Contexto

O01 materializó una transición compatible con `--path`, `session_dirs` y parsers nativos. O02 define el modelo objetivo: Backscroll debe ser TOML-only para ingesta canónica, con app config separada de input config y manifests concretos `*.inputs.toml`/`backscroll.inputs.d/*.toml` que describen discovery, decode, filtros, mapping, content y text normalization.

Antes de ejecutar O02/T002 y siguientes, las tasks activas deben reconciliarse con decisiones ya definidas en `docs/intention-agentic-input-definitions.md`, `docs/input-contract.md` y el README del Outcome O02. No se debe seguir especulando ni mezclar el modo transicional O01 con el objetivo O02.

## Alcance

**In**:
1. Revisar y actualizar O02/T002–T008, T010 y T012 para que reflejen el contrato final de inputs.
2. Explicitar que `--path`, `session_dirs` y fallback Claude implícito no pertenecen al flujo canónico O02 si contradicen TOML-only.
3. Explicitar que `backscroll.toml` es app config y los inputs viven en manifests concretos `*.inputs.toml`/`backscroll.inputs.d/*.toml`.
4. Reconciliar criterios de aceptación y verificaciones para que apunten al pipeline genérico `discover -> decode -> filter -> map -> emit`.
5. Alinear la policy de errores: manifests requeridos/activos inválidos fallan con error claro en flujos manifest-driven; validación/dry-run detallada queda en T010.
6. Actualizar dependencias `blocked_by` si hace falta para que no se ejecute una task ambigua antes de esta reconciliación.

**Out**:
- Implementar el input engine.
- Remover `--path` o `session_dirs` en código.
- Cambiar comportamiento runtime fuera de documentación/roadmap.

## Estado inicial esperado

- O02/T001 está Completed y existe `docs/input-contract.md`.
- O02/T002 sigue activa y puede interpretarse como breaking strict TOML-only, pero las tasks posteriores aún pueden contener ambigüedades heredadas del modo transicional.

## Criterios de Aceptación

- O02/T002–T008, T010 y T012 quedan implementables sin contradicciones con `docs/input-contract.md`.
- O02/T002 declara explícitamente la separación app config vs input config y la eliminación del flujo canónico basado en `--path`/`session_dirs`.
- O02/T008 declara explícitamente la remoción/refactor de APIs legacy del flujo principal sin dejar fallback Claude implícito.
- Ninguna task reconciliada introduce plugins/scripts ejecutables ni JMESPath como dependencia MVP.
- `rootline validate` pasa para todos los archivos modificados.
- `rootline graph docs/roadmap/ --where "isIndex == false" --check` no reporta ciclos ni links rotos.

## Fuente de verdad

- `docs/intention-agentic-input-definitions.md`
- `docs/input-contract.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/README.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T002-separate-app-config-from-input-config.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T003-implement-glob-discovery.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T004-implement-jsonl-jsonpath-engine.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T005-implement-declarative-filters-transforms.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T006-create-claude-input-preset.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T007-create-pi-input-preset.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T008-refactor-sync-read-api-to-input-engine.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T010-add-input-validation-dry-run.md`
- `docs/roadmap/O02-generic-agnostic-input-engine/T012-add-manifest-regression-tests.md`
