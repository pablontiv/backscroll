---
estado: Specified
tipo: task
---
# T004: Implement generic JSON/JSONL decoder and JSONPath selectors

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE1, CE3

[[blocked_by:./T003-implement-glob-discovery.md]]

## Preserva

- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: engine emite esas estructuras sin tocar DB.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: se usa JSONPath, no JMESPath.

## Contexto

El contrato final adopta selectores JSONPath sobre `serde_json::Value`. El pipeline de esta task cubre `decode -> record selector -> map/content selectors -> emit` para los formatos MVP `jsonl` y `json`.

Los filtros declarativos completos (`include_when`, `exclude_when`) y normalización de texto se completan en T005, pero esta task debe compilar/validar todos los selectores que ya estén presentes en el manifest.

## Alcance

**In**:
1. Agregar `serde_json_path` como dependencia.
2. Implementar `decode.format = "jsonl"` y `decode.format = "json"`, con `encoding = "utf-8"` como default MVP.
3. Compilar selectores JSONPath durante validación/carga de manifests activos.
4. Aplicar `record.selector` para elegir registros dentro del input decodificado.
5. Mapear campos declarativos de `[inputs.map]`: `role`, `uuid`, `timestamp`, `session_id`, `project` y `role_aliases`.
6. Seleccionar contenido desde `[inputs.content]`: `selector`, `string`, `blocks`, `block_text`, `content_type` y `default_content_type`.
7. Manejar errores de datos por línea/record sin abortar toda la sync, con conteos y diagnósticos; errores de manifest inválido fallan antes de sync.

**Out**:
- Operadores de filtros y text transforms; van en T005.
- Markdown/document sources; van en T009.
- JMESPath o lenguajes de expresión alternativos.

## Estado inicial esperado

- `parse_session_file_claude` usa `SessionRecord` tipado.
- `parse_pi_file` usa `serde_json::Value` con selectores hardcodeados.

## Criterios de Aceptación

- Un manifest JSONL simple produce `ParsedFile` y `ParsedMessage` desde selectors TOML.
- Un manifest JSON simple produce registros desde `record.selector`.
- Selectores inválidos fallan en validación, no a mitad de sync.
- Selectores ausentes o sin match tienen comportamiento definido por `docs/input-contract.md`.
- Tests cubren strings, objects, arrays básicos, role aliases y defaults de content type.

## Fuente de verdad

- `docs/input-contract.md`
- `src/core/sync.rs`
- `src/core/mod.rs`
- `src/core/models.rs`
- `Cargo.toml`
