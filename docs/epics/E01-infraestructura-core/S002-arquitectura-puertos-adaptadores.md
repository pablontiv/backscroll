---
estado: Completed
---
# S002: Arquitectura de Puertos y Adaptadores

**Estado:** Completed
**ID:** S002
**Parent:** E01

Refactorizar la estructura de carpetas para desacoplar el dominio (Core) de los detalles de implementación (Storage), siguiendo la investigación original.

## Tasks

- [x] `T005`: Crear directorios `src/core` y `src/storage`.
- [x] `T006`: Mover lógica de dominio (`domain.rs`, `models.rs`, `sync.rs`) a `src/core/`.
- [x] `T007`: Mover implementación de base de datos a `src/storage/sqlite.rs`.
- [x] `T008`: Actualizar módulos y visibilidad en `src/main.rs`.
