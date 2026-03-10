---
estado: Completed
---
# S001: Cargo Edition 2024

**Estado:** Completed
**ID:** S001
**Parent:** E01

Inicializar el proyecto Rust con la edición más reciente y las dependencias base necesarias para el motor de búsqueda y el parser.

## Tasks

- [x] `T001`: Ejecutar `cargo init --bin` y configurar `Cargo.toml` con Edition 2024.
- [x] `T002`: Añadir dependencias base: `rusqlite` (bundled), `serde`, `serde_json`, `clap` (derive), `miette`, `thiserror`.
- [x] `T003`: Configurar perfiles de release para optimización de tamaño y velocidad (LTO, codegen-units).
- [x] `T004`: Verificar compilación inicial con un "Hello Backscroll" básico.
