---
estado: Completed
tipo: task
---
# T001: Agregar dependencia refinery a Cargo.toml

**Outcome**: [Migrar setup_schema a refinery](README.md)

## Preserva

- Todas las dependencias existentes permanecen sin cambios.
- El binary sigue siendo self-contained (refinery no añade archivos externos en runtime).

## Contexto

Cargo.toml actualmente no tiene refinery. La feature rusqlite habilita la integración directa con rusqlite::Connection.

## Alcance

**In**:
1. Agregar refinery = { version = "0.8", features = ["rusqlite"] } a Cargo.toml.

**Out**:
1. Cambios a otras dependencias.
2. Cambios a Cargo.lock (se actualiza automáticamente).

## Estado inicial esperado

Cargo.toml sin refinery. Sistema de migraciones completamente manual.

## Criterios de Aceptación

- cargo build compila sin errores después de agregar refinery.
- refinery y refinery_core aparecen en Cargo.lock.

## Fuente de verdad

- Cargo.toml
