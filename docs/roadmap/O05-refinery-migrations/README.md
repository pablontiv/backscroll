---
tipo: outcome
---
# Migrar setup_schema a refinery

Reemplazar el sistema de migraciones hand-rolled de ~415 líneas en setup_schema() por refinery 0.8. V1 = schema v7 completo con IF NOT EXISTS. Transición legacy: detectar y eliminar tabla schema_version antes de correr refinery, sin pérdida de datos.
