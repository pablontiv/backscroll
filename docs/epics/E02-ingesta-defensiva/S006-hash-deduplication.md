---
estado: Completed
---
# S006: Sincronización Incremental (Hashes)

**Estado:** Completed
**ID:** S006
**Parent:** E02

Implementar lógica de ingesta eficiente mediante el uso de hashes de archivos para evitar re-indexación.

## Tasks

- [x] `T042`: Implementar `compute_hash` con `sha2`.
- [x] `T043`: Lógica de recorrido de archivos con `walkdir`.
- [x] `T044`: Tabla de control en la base de datos para almacenar hashes.
- [x] `T045`: Implementar lógica `is_file_changed` para omitir archivos ya indexados.
