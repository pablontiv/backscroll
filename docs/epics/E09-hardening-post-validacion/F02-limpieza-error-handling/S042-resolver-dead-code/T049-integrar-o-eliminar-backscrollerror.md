---
estado: Completed
tipo: refactor
ejecutable_en: 1 sesion
---
# T049: Integrar BackscrollError o eliminar

**Story**: [S042 Resolver dead code en errors.rs](README.md)
**Contribuye a**: P2 — zero dead code en errors.rs

## Preserva

- INV2: `just check` pasa
  - Verificar: `just check`
- INV3: Tests existentes no regresan
  - Verificar: `just test`

## Contexto

`src/errors.rs` define `BackscrollError` con 3 variantes marcadas `#[allow(dead_code)]`:
- `DatabaseOpen(String)` — con diagnostic code `backscroll::db_open_error`
- `ParseError(String)` — con diagnostic code `backscroll::parse_error`
- `PathNotFound { path: String }` — con diagnostic code `backscroll::io_error`

Ninguna se usa en el codebase. Todo error handling usa `miette::miette!()` o `.into_diagnostic()`.

## Especificacion Tecnica

**Opcion A (preferida): Eliminar**
1. Borrar `src/errors.rs`
2. Eliminar `mod errors;` de `src/main.rs`
3. Eliminar dependencia `thiserror` de `Cargo.toml` si no se usa en otro lugar

**Opcion B: Integrar**
Solo si las variantes aportan valor (diagnostic codes, help text). Reemplazar `.into_diagnostic()` en puntos clave por `BackscrollError::*`.

**Decision**: Evaluar si algun diagnostic code/help text aporta valor real para el usuario. Si no, opcion A.

## Alcance

**In**:
1. Decidir A o B
2. Ejecutar la opcion elegida
3. Verificar que no quedan imports huerfanos

**Out**: No cambiar logica de error handling existente (solo limpiar dead code).

## Estado inicial esperado

- `src/errors.rs` existe con `#[allow(dead_code)]`
- Todo compila y pasa tests

## Criterios de Aceptacion

- No existe `#[allow(dead_code)]` en el codebase
- `just check` pasa (zero warnings con clippy pedantic)
- `just test` pasa sin regresiones
- Si opcion A: `src/errors.rs` no existe y `mod errors` eliminado de main.rs

## Fuente de verdad

- `src/errors.rs` (antes)
- `src/main.rs` — declaracion de modulos
- `Cargo.toml` — dependencia thiserror
