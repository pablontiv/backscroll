---
estado: Completed
---
# S008: FTS5 Optimizer (BM25)

**Estado:** Completed
**ID:** S008
**Parent:** E03

Configurar y optimizar el motor de búsqueda SQLite FTS5 con soporte para ranking BM25.

## Tasks

- [x] `T050`: Crear tabla virtual `messages_fts` con tokenizador `unicode61`.
- [x] `T051`: Implementar consultas `MATCH` con ordenamiento por `rank`.
- [x] `T052`: Filtrado opcional por campo `project`.
- [x] `T053`: Optimizar esquema para consultas de baja latencia.
