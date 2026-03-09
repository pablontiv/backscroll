---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T062: Plan parser function

**Story**: [S047 Markdown plan parser](README.md)
**Contribuye a**: P1 (plans indexados), P2 (plans spliteados por headers)

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Necesitamos parsear archivos markdown de plans y splitear por `##` headers para indexar secciones individuales.

## Especificacion Tecnica

Crear nuevo modulo `src/core/plans.rs`:

1. `pub fn parse_plan(path: &Path) -> miette::Result<ParsedFile>`
2. Leer archivo completo como string
3. Splitear por regex `^## ` (heading level 2)
4. Cada seccion → un `ParsedMessage`:
   - `role`: "plan"
   - `text`: contenido de la seccion (incluyendo heading)
   - `ordinal`: indice secuencial
   - `uuid`: None (plans no tienen UUID)
   - `timestamp`: None (o file mtime)
5. Si no hay `##` headers, todo el archivo es una sola seccion
6. Computar SHA-256 hash del archivo completo
7. Retornar `ParsedFile` con `source_path`, `hash`, `project: None`, `messages`

Agregar `pub mod plans;` en `src/core/mod.rs`.

## Alcance

**In**: Nuevo modulo plans.rs con parse_plan()
**Out**: No integrar con sync (T065), no agregar source field (T064)

## Criterios de Aceptacion

- `parse_plan()` retorna ParsedFile con secciones correctas
- Split por `##` funciona
- Sin `##` → una sola seccion
- `just check` pasa

## Fuente de verdad

- `src/core/plans.rs` — nuevo modulo
