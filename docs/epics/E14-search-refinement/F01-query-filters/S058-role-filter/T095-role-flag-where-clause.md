---
ejecutable_en: 1 sesion
estado: Pending # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T095: Add --role flag and implement role WHERE clause

**Story**: [S058 Role Filter](README.md)
**Contribuye a**: --role flag filtra por rol en search

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

La tabla search_items tiene columna `role TEXT NOT NULL` con valores "user" y "assistant". El CLI debe aceptar `--role human|assistant` (nota: "human" se mapea a "user" en el DB). El patron es identico al de project/source filtering: agregar condicion `si.role = ?` al vector conditions.

Se extiende el SearchEngine trait con un parametro `role: &Option<String>` analogamente a como se hizo con after/before en T092.

## Alcance

**In**:
1. Agregar `--role` como `Option<String>` al struct Search en main.rs
2. Extender SearchEngine::search() con parametro `role: &Option<String>`
3. Implementar WHERE clause `si.role = ?` en sqlite.rs
4. Mapear "human" → "user" en la query (el JSONL usa "user", el flag usa "human" por ergonomia)

**Out**: Tests (T096)

## Estado inicial esperado

- SearchEngine::search() con parametros extendidos (post-T092)
- search() en sqlite.rs con patron conditions.push()

## Criterios de Aceptacion

- `backscroll search "test" --role human` retorna solo mensajes con role="user"
- `backscroll search "test" --role assistant` retorna solo mensajes con role="assistant"
- Sin --role, retorna ambos roles
- `--role invalid` produce error descriptivo

## Fuente de verdad

- `src/main.rs` — Search command struct
- `src/core/mod.rs` — SearchEngine trait
- `src/storage/sqlite.rs` — search() WHERE construction
