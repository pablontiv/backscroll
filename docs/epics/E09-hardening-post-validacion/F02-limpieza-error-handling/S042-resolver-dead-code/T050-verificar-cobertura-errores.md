---
estado: Completed
tipo: test
ejecutable_en: 1 sesion
---
# T050: Verificar cobertura de errores en flujos

**Story**: [S042 Resolver dead code en errors.rs](README.md)
**Contribuye a**: P2 — zero dead code, error handling coherente

[[blocks:T049-integrar-o-eliminar-backscrollerror]]

## Preserva

- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Tests existentes no regresan
  - Verificar: `just test`

## Contexto

Despues de T049, verificar que el refactor no dejo imports huerfanos, warnings de compilacion, ni regresiones en tests.

## Especificacion Tecnica

1. Ejecutar `just check` — debe pasar sin warnings
2. Ejecutar `just test` — todos los tests pasan
3. Buscar imports residuales: `grep -r "errors" src/` no retorna referencias a modulo eliminado
4. Si opcion A (eliminacion): verificar que `thiserror` no se importa en ningun otro modulo
5. Si opcion B (integracion): verificar que los diagnostic codes aparecen en output de errores

## Alcance

**In**:
1. Verificacion post-refactor de T049
2. Documentar resultado

**Out**: No cambiar codigo (solo verificar).

## Estado inicial esperado

- T049 completado (BackscrollError integrado o eliminado)

## Criterios de Aceptacion

- `just check` pasa
- `just test` pasa
- No existen imports huerfanos de `errors` module
- No existen `#[allow(dead_code)]` en el codebase

## Fuente de verdad

- Output de `just check` y `just test`
