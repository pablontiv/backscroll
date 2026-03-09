---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T046: Agregar 3 patrones regex faltantes a filter_noise()

**Story**: [S040 Patrones de ruido faltantes](README.md)
**Contribuye a**: P1 — todos los patrones de ruido del research filtrados

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: `just test` (tests de parse existentes pasan)
- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Tests existentes no regresan
  - Verificar: `just test`

## Contexto

La validacion research-vs-implementacion detecto 3 patrones de ruido documentados en `docs/research/backscroll-session-search-cli.md` (seccion "Patrones de ruido verificados") que no estan en `filter_noise()`:

1. `<local-command-stdout>` tags — output de hooks locales (frecuentemente vacio)
2. `<command-name>`, `<command-message>`, `<command-args>` tags — command XML adicionales
3. `Caveat:` prefix standalone — lineas de caveat sin wrapper XML

## Especificacion Tecnica

Agregar al array `tags_to_remove` en `filter_noise()` (`src/core/sync.rs`):

```rust
// Hook stdout blocks
r"<local-command-stdout>[\s\S]*?</local-command-stdout>",
// Command metadata tags
r"<command-name>[\s\S]*?</command-name>",
r"<command-message>[\s\S]*?</command-message>",
r"<command-args>[\s\S]*?</command-args>",
```

Agregar patron de linea (junto a "Base directory:"):
```rust
// Caveat prefix lines
r"(?m)^Caveat:.*$"
```

## Alcance

**In**:
1. Agregar 5 regex patterns nuevos a `filter_noise()`
2. Mantener estructura existente (array + loop)

**Out**: No cambiar la arquitectura de filter_noise (LazyLock es T048).

## Estado inicial esperado

- `filter_noise()` funciona con 7 patrones existentes
- Tests existentes pasan

## Criterios de Aceptacion

- Los 5 patterns nuevos estan en `tags_to_remove` o como regex adicionales
- `just check` pasa
- Tests existentes no regresan

## Fuente de verdad

- `src/core/sync.rs` — funcion `filter_noise()`
