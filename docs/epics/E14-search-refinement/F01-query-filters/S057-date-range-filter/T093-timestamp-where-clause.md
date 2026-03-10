---
ejecutable_en: 1 sesion
estado: Completed # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T093: Implement timestamp WHERE clause in SQLite search()

**Story**: [S057 Date Range Filter](README.md)
**Contribuye a**: Search filtra resultados por timestamp en la query SQL

[[blocks:T092-date-flags-trait]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

La tabla search_items tiene columna `timestamp TEXT`. Los valores son strings numericos (unix epoch) o ISO 8601. La query actual en sqlite.rs:225-280 construye condiciones dinamicas con `conditions.push()` y `params.push()` para project y source. Se necesita agregar condiciones analogas para timestamp.

Nota: timestamp en search_items puede ser null para algunos records. El WHERE debe usar `si.timestamp IS NOT NULL AND si.timestamp >= ?` para evitar comparar con null.

## Alcance

**In**:
1. En `Database::search()`, agregar condiciones para after/before usando `si.timestamp >= ?` y `si.timestamp < ?`
2. Push parametros correspondientes al vector de params
3. Manejar el caso donde timestamp es null (filtrar implicitamente records sin timestamp)

**Out**: CLI flags (T092), tests (T094)

## Estado inicial esperado

- T092 completado (search() acepta parametros after/before)
- search() en sqlite.rs con patron de conditions.push() existente

## Criterios de Aceptacion

- `backscroll search "test" --after 2026-03-01` retorna solo resultados con timestamp >= "2026-03-01"
- `backscroll search "test" --before 2026-03-09` retorna solo resultados con timestamp < "2026-03-09"
- Records con timestamp NULL son excluidos cuando se usa --after o --before
- Sin flags temporales, query SQL es identica a la actual

## Fuente de verdad

- `src/storage/sqlite.rs:225-280` — search() implementation
