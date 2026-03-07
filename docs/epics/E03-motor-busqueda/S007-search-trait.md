# S007: SearchEngine Trait

**Estado:** Pending
**ID:** S007
**Parent:** E03

Definir la interfaz de dominio para garantizar que el motor de búsqueda pueda ser intercambiable en el futuro.

## Tasks

- [ ] `T020`: Definir el Trait `SearchEngine` en `core/domain.rs`.
- [ ] `T021`: Implementar las estructuras de datos `SearchResult` y `SearchQuery`.
- [ ] `T022`: Instrumentar el Trait con Spans de `tracing`.
