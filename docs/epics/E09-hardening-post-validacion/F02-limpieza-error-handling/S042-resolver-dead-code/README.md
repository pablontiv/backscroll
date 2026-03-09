# S042: Resolver dead code en errors.rs

**Feature**: [F02 Limpieza de Error Handling](../README.md)
**Capacidad**: `BackscrollError` enum se integra en el codebase o se elimina. Zero `#[allow(dead_code)]`.
**Cubre**: P2 del Epic (zero dead code)

## Antes / Despues

**Antes**: `src/errors.rs` define `BackscrollError` con 3 variantes (DatabaseOpen, ParseError, PathNotFound) marcadas `#[allow(dead_code)]`. Ningun modulo las usa — todo el codebase usa `miette::miette!()` directamente.

**Despues**: Dead code eliminado. El modulo `errors` se elimina si las variantes no aportan valor, o se integra como error type en los flujos de sync/config/sqlite si aporta mejor diagnostico.

## Criterios de Aceptacion (semanticos)

- [ ] No existe `#[allow(dead_code)]` en el codebase
- [ ] `just check` pasa sin warnings
- [ ] `just test` pasa sin regresiones

## Invariantes

- INV2: `just check` pasa
- INV3: Tests existentes no regresan

## Tasks

| Task | Descripcion |
|------|-------------|
| [T049](T049-integrar-o-eliminar-backscrollerror.md) | Integrar BackscrollError o eliminar |
| [T050](T050-verificar-cobertura-errores.md) | Verificar cobertura de errores en flujos |

## Fuente de verdad

- `src/errors.rs` — enum BackscrollError
- `src/main.rs` — modulo declaration `mod errors`
