---
estado: Specified
tipo: task
---
# T004: Diseñar trait/registry de `SessionInputParser`

**Outcome**: [O01 Refactor de parser de sesiones para inputs declarativos](README.md)
**Contribuye a**: Habilitar parseres externos futuros sin modificar la pipeline central.

## Preserva

- INV1: Mantener frontera interna de ingestión con `ParsedFile` y `ParsedMessage`.
  - Verificar: adapters sólo emiten estructuras actuales.

## Contexto

Se necesita una interfaz interna que registre distintos orígenes (`claude`, `pi`, etc.) y resuelva parseo por configuración.

## Alcance

**In**:
1. Definir trait con método(s) de parseo y metadatos.
2. Implementar registry con selección por `source`.
3. Definir soporte de rutas activas/inactivas y manejo de errores por archivo.

**Out**:
- Comenzar soporte a adapters ejecutables o de comando.

## Estado inicial esperado

- `parse_sessions` procesa archivos de forma monolítica sin registro de parsers.

## Criterios de Aceptación

- Existe estructura de dispatch limpia y testeable.
- Se agrega prueba unitaria de selección de parser por `source`.
- Error en un archivo no rompe todo el lote de sync.

## Fuente de verdad

- `src/core/sync.rs`
- `src/main.rs`
- Tests relacionados de sync.
