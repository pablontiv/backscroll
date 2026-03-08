# S038: Metricas reales del indice

**Feature**: [F03 Status y Documentacion](../README.md)
**Capacidad**: Comando status muestra metricas reales: conteo de archivos, mensajes, tamano de DB, ultimo sync.
**Cubre**: P3 del Epic (status con metricas reales)

## Antes / Despues

**Antes**: `backscroll status` es un stub que imprime "OK" hardcodeado. No consulta la base de datos.

**Despues**: Status ejecuta queries reales: `count(DISTINCT source_path)` archivos, `count(*)` mensajes, `PRAGMA page_count * page_size` tamano, `max(timestamp)` ultimo sync. Output formateado con metricas.

## Criterios de Aceptacion (semanticos)

- [x] `backscroll status` muestra conteo de archivos indexados
- [x] `backscroll status` muestra conteo de mensajes
- [x] `backscroll status` muestra tamano de la DB

## Invariantes

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` no se ve afectado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll status` < 1s

## Tasks

| Task | Descripcion |
|------|-------------|
| [T042](T042-status-queries.md) | Queries de metricas del indice |
| [T043](T043-status-format.md) | Formatear status con metricas, test snapshot |

## Fuente de verdad

- `src/storage/sqlite.rs` — queries de metricas
- `src/main.rs` — comando status
