---
estado: Pending
tipo: refactor
ejecutable_en: 1 sesion
---
# T017: Remover #[allow(dead_code)], verificar clippy limpio

**Story**: [S026 Refactorizar sync y main](README.md)
**Contribuye a**: Zero #[allow(dead_code)] en src/

[[blocks:T016-main-dyn-dispatch]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Con el trait implementado y usado via dyn dispatch, no deberia haber dead code. Eliminar todos los `#[allow(dead_code)]` y verificar que clippy pasa limpio.

## Alcance

**In**:
1. Remover todos los `#[allow(dead_code)]` de src/
2. Remover metodos legacy de Database que fueron reemplazados por impl SearchEngine
3. Verificar clippy limpio

**Out**: No agregar funcionalidad nueva.

## Estado inicial esperado

- main.rs usa dyn SearchEngine (T016)
- Puede haber `#[allow(dead_code)]` residuales

## Criterios de Aceptacion

- `grep -r "allow(dead_code)" src/` retorna 0 resultados
- `just check` pasa (incluye clippy -D warnings)

## Fuente de verdad

- Todos los archivos en `src/`
