---
estado: Completed
tipo: task
---
# T008: Preservar semántica de ingestión existente

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: No degradar resultados actuales para sesiones.

## Preserva

- INV1: Mantener estructura de mensajes e índice actual para sesiones.
  - Verificar: búsquedas regresivas con textos conocidos continúan.

## Contexto

Antes de introducir la capa declarativa, se deben codificar invariantes funcionales: tipo de mensajes, noise filter, uuid/path/hash, proyecto detectado, exclusión de agentes por defecto.

## Alcance

**In**:
1. Testear igualdad funcional de parseo entre ruta legacy y nueva arquitectura.
2. Asegurar fallback de `source="session"` con hash/uuid/source_path.
3. Mantener `source_path` como clave principal en indexed_files.

**Out**:
- Modificar reglas de búsqueda/score o schema.

## Estado inicial esperado

- No hay pruebas explícitas de equivalencia entre ruta antigua y nueva pipeline.

## Criterios de Aceptación

- Test de compatibilidad de mensajes parseados pasa.
- `sync` existente no elimina casos de ruido ya filtrado.
- Deduplicación sigue por hash funcional.

## Fuente de verdad

- `src/core/sync.rs`
- `src/storage/sqlite.rs`
- `tests/cli.rs`
