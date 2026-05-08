---
estado: Specified
tipo: task
---
# T004: Implement generic JSONL decoder and JSONPath selectors

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE1, CE3

[[blocked_by:./T003-implement-glob-discovery.md]]

## Preserva

- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: engine emite esas estructuras sin tocar DB.
- INV4: JMESPath queda como evaluación futura, no dependencia del MVP.
  - Verificar: se usa JSONPath, no JMESPath.

## Contexto

La evidencia externa sugiere usar selectores declarativos. Para el MVP se adopta JSONPath con `serde_json_path` por ser estándar RFC 9535 y operar sobre `serde_json::Value`.

## Alcance

**In**:
1. Agregar `serde_json_path` como dependencia.
2. Implementar `decode.format = "jsonl"`.
3. Compilar selectores JSONPath durante validación.
4. Mapear campos declarativos a `role`, `content`, `uuid`, `timestamp`, `project` si aplica.
5. Manejar errores por línea sin abortar toda la sync, con conteos/diagnósticos.

**Out**:
- Filtros complejos y transforms de contenido; van en T005.
- Markdown/document sources; van en T009.

## Estado inicial esperado

- `parse_session_file_claude` usa `SessionRecord` tipado.
- `parse_pi_file` usa `serde_json::Value` con selectores hardcodeados.

## Criterios de Aceptación

- Un manifest JSONL simple produce `ParsedFile` y `ParsedMessage` desde selectors TOML.
- Selectores inválidos fallan en validación, no a mitad de sync.
- Selectores ausentes o sin match tienen comportamiento definido por el contrato.
- Tests cubren strings, objects y arrays básicos.

## Fuente de verdad

- `src/core/sync.rs`
- `src/core/mod.rs`
- `src/core/models.rs`
- `Cargo.toml`
