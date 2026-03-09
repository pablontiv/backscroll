# S041: Optimizacion de compilacion regex

**Feature**: [F01 Completitud de Filtros de Ruido](../README.md)
**Capacidad**: Regex patterns se compilan una sola vez usando `LazyLock` en vez de recompilarse en cada llamada a `filter_noise()`.
**Cubre**: P3 del Epic (regex compilados una vez)

## Antes / Despues

**Antes**: `filter_noise()` compila 8+ regex patterns con `Regex::new()` en cada invocacion. Para un sync de 500 archivos con ~20 mensajes cada uno, son ~80,000 compilaciones de regex innecesarias.

**Despues**: Todos los patterns viven en un `static LazyLock<Vec<Regex>>` compilado una sola vez al primer uso. Performance de sync mejora proporcionalmente al numero de mensajes procesados.

## Criterios de Aceptacion (semanticos)

- [ ] `filter_noise()` no contiene llamadas a `Regex::new()`
- [ ] Patterns declarados como `static LazyLock<Vec<Regex>>`
- [ ] Todos los tests de filtrado siguen pasando

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales
- INV2: `just check` pasa
- INV3: Tests existentes no regresan

## Tasks

| Task | Descripcion |
|------|-------------|
| [T048](T048-lazylock-regex.md) | Migrar regex a LazyLock |

## Fuente de verdad

- `src/core/sync.rs` — funcion `filter_noise()` y static patterns
