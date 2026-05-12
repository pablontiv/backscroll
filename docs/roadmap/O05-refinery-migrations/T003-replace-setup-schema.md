---
estado: Completed
tipo: task
---
# T003: Reemplazar cuerpo de setup_schema() en sqlite.rs

**Outcome**: [Migrar setup_schema a refinery](README.md)

## Preserva

- La interfaz pública de Database no cambia más allá de la firma de setup_schema.
- Todos los callers de setup_schema siguen compilando (ajuste let mut si es necesario).
- El comportamiento observable de la DB es idéntico.

## Contexto

setup_schema() actualmente tiene ~415 líneas con 7 bloques de migración manual. Cada bloque re-consulta la versión, gestiona transacciones y actualiza schema_version. Todo esto se reemplaza por una llamada delegada.

## Alcance

**In**:
1. Cambio de firma &self a &mut self en setup_schema.
2. Eliminación de las ~415 líneas del cuerpo.
3. Sustitución por migrations::run(&mut self.conn).

**Out**:
1. Cambios a otros métodos de Database.
2. Cambios a open() o open_readonly().

## Estado inicial esperado

setup_schema() tiene ~415 líneas de lógica de migraciones manual con tabla schema_version.

## Criterios de Aceptación

- setup_schema() tiene cuerpo de 1 línea delegando a migrations::run().
- No existe ninguna referencia a schema_version en sqlite.rs fuera de comentarios históricos.
- cargo build compila sin errores.

## Fuente de verdad

- src/storage/sqlite.rs
