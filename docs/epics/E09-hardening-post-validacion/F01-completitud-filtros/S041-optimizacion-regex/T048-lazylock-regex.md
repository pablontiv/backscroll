---
estado: Completed
tipo: refactor
ejecutable_en: 1 sesion
---
# T048: Migrar regex a LazyLock

**Story**: [S041 Optimizacion de compilacion regex](README.md)
**Contribuye a**: P3 — regex compilados una sola vez

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: `just test`
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Tests existentes no regresan
  - Verificar: `just test`

## Contexto

`filter_noise()` compila 10+ regex patterns con `Regex::new()` en cada invocacion. En un sync tipico con miles de mensajes, esto genera miles de compilaciones redundantes. `std::sync::LazyLock` (estable desde Rust 1.80) permite compilar una vez.

## Especificacion Tecnica

Reemplazar la compilacion inline por un static:

```rust
use std::sync::LazyLock;
use regex::Regex;

static NOISE_TAG_PATTERNS: LazyLock<Vec<Regex>> = LazyLock::new(|| {
    [
        r"<system-reminder>[\s\S]*?</system-reminder>",
        r"<task-notification>[\s\S]*?</task-notification>",
        r"<caveat>[\s\S]*?</caveat>",
        r"<local-command-caveat>[\s\S]*?</local-command-caveat>",
        r"<local-command-stdout>[\s\S]*?</local-command-stdout>",
        r"<command>[\s\S]*?</command>",
        r"<command-name>[\s\S]*?</command-name>",
        r"<command-message>[\s\S]*?</command-message>",
        r"<command-args>[\s\S]*?</command-args>",
    ]
    .iter()
    .map(|p| Regex::new(p).expect("invalid noise pattern"))
    .collect()
});

static NOISE_LINE_PATTERNS: LazyLock<Vec<Regex>> = LazyLock::new(|| {
    [
        r"(?m)^Base directory:.*$",
        r"(?m)^Caveat:.*$",
    ]
    .iter()
    .map(|p| Regex::new(p).expect("invalid noise line pattern"))
    .collect()
});
```

Actualizar `filter_noise()` para iterar sobre los statics en vez de compilar inline.

## Alcance

**In**:
1. Crear statics `NOISE_TAG_PATTERNS` y `NOISE_LINE_PATTERNS`
2. Refactorizar `filter_noise()` para usarlos
3. Eliminar `if let Ok(re) = Regex::new(...)` — los patterns se validan en compile-time via `.expect()`

**Out**: No cambiar logica de filtrado (solo mover compilacion).

## Estado inicial esperado

- T046 completado (todos los patterns ya estan en filter_noise)
- Tests de filtrado pasan

## Criterios de Aceptacion

- `filter_noise()` no contiene `Regex::new()`
- Patterns declarados como `static LazyLock<Vec<Regex>>`
- `just check` pasa
- `just test` pasa (todos los tests de filtrado)

## Fuente de verdad

- `src/core/sync.rs` — statics y funcion `filter_noise()`
