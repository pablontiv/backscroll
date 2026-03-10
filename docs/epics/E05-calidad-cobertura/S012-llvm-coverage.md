---
estado: Completed
---
# S012: LLVM Coverage Gate (85%)

**Estado:** Completed
**ID:** S012
**Parent:** E05

Establecer un umbral mínimo de calidad de código mediante la cobertura de pruebas automatizada.

## Tasks

- [x] `T040`: Integrar `cargo-llvm-cov` en el flujo de CI de GitHub Actions.
- [x] `T041`: Configurar el gate para bloquear PRs que no alcancen el 85% de cobertura.
- [x] `T042`: Implementar el reporte detallado de cobertura en el comentario del PR.
