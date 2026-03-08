---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T042: Queries de metricas del indice

**Story**: [S038 Metricas reales del indice](README.md)
**Contribuye a**: backscroll status muestra conteo de archivos y mensajes

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` no se ve afectado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll status` < 1s

## Contexto

El comando status es un stub. Se necesitan queries reales contra la DB para reportar metricas utiles: cuantos archivos, cuantos mensajes, tamano de la DB, ultimo sync.

## Especificacion Tecnica

```sql
-- Archivos indexados
SELECT count(DISTINCT source_path) FROM search_items;

-- Mensajes totales
SELECT count(*) FROM search_items;

-- Tamano de DB
PRAGMA page_count;
PRAGMA page_size;
-- tamano = page_count * page_size

-- Ultimo sync
SELECT max(timestamp) FROM search_items;

-- Proyectos distintos
SELECT count(DISTINCT project) FROM search_items;
```

## Alcance

**In**:
1. Agregar metodo `get_stats() -> Stats` a SearchEngine trait o directamente a Database
2. Struct Stats con: file_count, message_count, db_size_bytes, last_sync, project_count
3. Implementar queries

**Out**: No formatear output (T043).

## Estado inicial esperado

- search_items table con datos (E06/E07 completados)
- Status es un stub

## Criterios de Aceptacion

- `cargo test test_get_stats` — retorna metricas correctas en DB de test
- Stats tiene todos los campos: file_count, message_count, db_size_bytes, last_sync, project_count
- `just check` pasa

## Fuente de verdad

- `src/storage/sqlite.rs`
