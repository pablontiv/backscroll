---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T025: Tests unitarios para cada patron con fixtures

**Story**: [S030 Filtrado de patrones de ruido](README.md)
**Contribuye a**: tool_use/tool_result blocks no se indexan como texto

[[blocks:T024-noise-filters]]

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Cada filtro de ruido necesita un test unitario dedicado con fixture que demuestre que el patron se filtra correctamente, y que contenido no-ruidoso se preserva.

## Alcance

**In**:
1. Test para cada uno de los 8+ patrones de ruido
2. Test de mensaje mixto (ruido + contenido util) → contenido util preservado
3. Test de mensaje 100% ruido → None
4. Test de mensaje limpio → sin cambios

**Out**: No agregar nuevos filtros.

## Estado inicial esperado

- filter_noise implementada con 8+ filtros (T024)

## Criterios de Aceptacion

- `cargo test test_noise_` — al menos 8 tests nombrados con patron especifico
- Cada test verifica tanto filtering como preservation de contenido util
- `just check` pasa

## Fuente de verdad

- Tests unitarios en modulo de filtrado
