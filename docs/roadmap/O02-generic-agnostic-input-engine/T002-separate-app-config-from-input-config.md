---
estado: Completed
tipo: task
---
# T002: Separate app config from input config

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE2

[[blocked_by:./T001-define-generic-input-contract.md]]
[[blocked_by:./T014-reconcile-roadmap-with-final-input-contract.md]]

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: los manifests de input conservan `source = "session"` para Claude/Pi; app config no redefine fuentes de ingesta.
- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: esta task solo separa carga/configuración previa a parseo.

## Contexto

O01 fue una transición compatible con `--path`, `session_dir(s)` y discovery Claude implícito. O02 define el modelo canónico TOML-only: `backscroll.toml` es configuración de aplicación, mientras que la ingesta vive en manifests concretos `*.inputs.toml` y/o `backscroll.inputs.d/*.toml` que cumplen `docs/input-contract.md`.

Por lo tanto, `--path`, `session_dir`, `session_dirs` y fallback Claude implícito pueden existir solo como compatibilidad/migración explícita fuera del flujo canónico. No deben alimentar silenciosamente la ingesta principal de O02.

## Alcance

**In**:
1. Crear un modelo de input config separado de `Config`, basado en el contrato `version = 1` + `[[inputs]]`.
2. Cargar manifests `*.inputs.toml`/`backscroll.inputs.d/*.toml` desde las ubicaciones definidas por T001/T014.
3. Mantener en app config solo configuración global de Backscroll, como `database_path`, embedding y opciones no relacionadas con discovery de inputs.
4. Remover del camino canónico cualquier resolución de ingesta basada en `--path`, `session_dir`, `session_dirs` o discovery Claude implícito.
5. Definir una política explícita para compatibilidad legacy: si se conserva, debe estar aislada, documentada como no canónica y no activarse como fallback silencioso.
6. Actualizar docs para describir la separación app config vs input config.

**Out**:
- Implementar el engine JSONL/JSONPath completo.
- Migrar plans/external sources completos.
- Crear compatibilidad legacy nueva que contradiga TOML-only.

## Estado inicial esperado

- `docs/input-contract.md` existe y define `version`, `[[inputs]]`, `discover`, `decode`, `record`, `map`, `content` y `text`.
- `src/config.rs` todavía mezcla rutas de ingesta históricas con configuración de aplicación.
- El CLI todavía puede resolver sesiones desde flags/config legacy heredados de O01.

## Criterios de Aceptación

- `backscroll.toml` no es fuente primaria ni canónica de rutas de ingesta.
- Los inputs se cargan desde manifests `*.inputs.toml`/`backscroll.inputs.d/*.toml` y los manifests requeridos/activos inválidos fallan con error claro.
- No hay `--path`, `session_dir`, `session_dirs` ni fallback Claude alimentando silenciosamente el parser de sesiones en el flujo canónico.
- App config e input config tienen tipos, loaders y tests separados.
- La documentación declara explícitamente que O01 fue transicional y que O02 usa manifests TOML como contrato canónico de ingesta.

## Fuente de verdad

- `docs/input-contract.md`
- `docs/intention-agentic-input-definitions.md`
- `src/config.rs`
- `src/main.rs`
- `docs/configuration.md`
- `backscroll.toml.example`
