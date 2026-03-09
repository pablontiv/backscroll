# S047: Markdown plan parser

**Feature**: [F01 Plan Parser & Sync](../README.md)
**Capacidad**: Una funcion parsea un archivo markdown y retorna un `Vec<ParsedMessage>` donde cada entrada corresponde a una seccion `##` (heading como descriptor, body como text).
**Cubre**: P1 (plans indexados), P2 (plans spliteados por headers)

## Antes / Despues

**Antes**: Solo archivos JSONL de sesion se parsean. No hay capacidad de parsing de markdown.

**Despues**: `parse_plan(path) -> ParsedFile` splitea por headings `##`, produce un ParsedMessage por seccion con `role = "plan"`, `text = contenido de seccion incluyendo heading`.

## Criterios de Aceptacion (semanticos)

- [ ] Plan de una sola seccion parseado correctamente
- [ ] Plan multi-seccion produce un ParsedMessage por `##` heading
- [ ] Plan sin `##` headers se trata como una sola seccion (archivo completo)
- [ ] Plan vacio no produce error

## Invariantes

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Tasks

| Task | Descripcion |
|------|-------------|
| [T062](T062-plan-parser-function.md) | Plan parser function |
| [T063](T063-plan-parser-tests.md) | Plan parser tests |

## Fuente de verdad

- `src/core/plans.rs` — nuevo modulo parse_plan()
