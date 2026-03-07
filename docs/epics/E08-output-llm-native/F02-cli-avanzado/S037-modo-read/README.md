# S037: Modo --read

**Feature**: [F02 CLI Avanzado](../README.md)
**Capacidad**: Subcomando `read` para lectura filtrada de sesiones JSONL individuales.
**Cubre**: Extiende CLI con modo de lectura directa (research v1 capability M5)

## Antes / Despues

**Antes**: Solo 3 comandos: sync, search, status. No hay forma de leer una sesion individual filtrada. Para ver una sesion hay que usar jq manualmente.

**Despues**: `backscroll read <session-path>` muestra contenido de una sesion JSONL con noise filtering aplicado. Solo mensajes user/assistant, formateados legiblemente.

## Criterios de Aceptacion (semanticos)

- [ ] `backscroll read session.jsonl` muestra contenido filtrado
- [ ] Noise filtering se aplica al leer

## Invariantes

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` no se ve afectado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll read test.jsonl` < 1s

## Tasks

| Task | Descripcion |
|------|-------------|
| [T040](T040-reader-module.md) | Crear core/reader.rs para lectura filtrada |
| [T041](T041-read-subcommand.md) | Implementar subcomando read con noise filtering |

## Fuente de verdad

- `src/core/reader.rs` — modulo de lectura (nuevo)
- `src/main.rs` — CLI subcommand
