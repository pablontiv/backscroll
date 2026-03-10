---
estado: Completed
---
# S004: SQLite WAL (Persistencia Robusta)

**Estado:** Completed
**ID:** S004
**Parent:** E01

Configurar la base de datos SQLite para soportar concurrencia y robustez en la persistencia.

## Tasks

- [x] `T034`: Implementar `src/db.rs` con `rusqlite::Connection`.
- [x] `T035`: Configurar `PRAGMA journal_mode=WAL` y `busy_timeout`.
- [x] `T036`: Integrar conexión segura a la DB en el arranque.
- [x] `T037`: Implementar esquema básico de control (`indexed_files`).
