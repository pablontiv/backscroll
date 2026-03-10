---
ejecutable_en: 1 sesion
estado: Completed # [Pending, Specified, In Progress, Completed, Blocked, On Hold, Obsolete]
tipo: code # [code, test, refactor, chore, docs]
---
# T097: Replace LIMIT 20 with --limit/--offset flags

**Story**: [S059 Configurable Pagination](README.md)
**Contribuye a**: Search acepta --limit y --offset con LIMIT 20 como default

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV4: --limit default = 20
  - Verificar: `backscroll search "test"` retorna max 20 resultados (sin flag)

## Contexto

En sqlite.rs:260-263, la query tiene `LIMIT 20` hard-coded. Se necesita:
1. Agregar `--limit` y `--offset` al Search command en main.rs
2. Extender SearchEngine::search() con parametros limit y offset
3. Reemplazar `LIMIT 20` con `LIMIT ? OFFSET ?` parametrizados

Default de --limit es 20 para backward compatibility. --limit 0 significa sin limite.

## Alcance

**In**:
1. Agregar `--limit` (default 20) y `--offset` (default 0) al Search struct
2. Extender SearchEngine::search() signature con limit/offset
3. Reemplazar LIMIT hard-coded con parametros SQL
4. --limit 0 → sin LIMIT clause

**Out**: Tests (T098)

## Estado inicial esperado

- sqlite.rs con LIMIT 20 en lineas 260 y 263
- SearchEngine trait (ya extendido con after/before/role en T092/T095)

## Criterios de Aceptacion

- `backscroll search "test" --limit 5` retorna maximo 5 resultados
- `backscroll search "test" --limit 50` retorna hasta 50
- `backscroll search "test"` retorna maximo 20 (default, backward compat)
- `backscroll search "test" --offset 10` salta los primeros 10

## Fuente de verdad

- `src/main.rs` — Search struct
- `src/storage/sqlite.rs:260-263` — LIMIT 20
