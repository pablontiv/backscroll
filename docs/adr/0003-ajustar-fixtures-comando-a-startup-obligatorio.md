---
tipo: adr
estado: accepted
fecha: "2026-08-21"
contexto: "La política de arranque obligatorio ejecuta preparación y sync antes de handlers. Varios tests de cmd/backscroll seguían afirmando contratos previos (lectura sin side effects, diagnóstico WAL en status/validate, errores impresos en stderr con SilenceErrors activo), provocando fallas y un timeout por inputs ambientales en annotate."
decision: "Reescribir los tests de comando para validar el contrato posterior al arranque obligatorio: aislar HOME/config/inputs en pruebas que invocan run, aceptar preparación de índice en startup para status/validate/config, validar errores por valor retornado cuando Cobra silencia stderr y reservar invariantes de no mutación estricta para pruebas directas cuando corresponda."
consecuencias: "El paquete cmd/backscroll deja de colgar por auto-sync ambiental y las fallas restantes quedan concentradas en contratos de documentación/skill (Task 7). La suite de comandos refleja el comportamiento real del CLI con startup obligatorio y reduce falsos rojos por expectativas de snapshot obsoletas."
---

# Ajustar fixtures de comando a startup obligatorio

## Contexto

Task 6 migra fixtures históricas que dependían de `--indexed-only` y de expectativas de lectura sin arranque. Tras el cambio arquitectónico, cada comando operativo pasa por `PersistentPreRunE`, que prepara índice compatible e intenta sync incremental antes del handler.

Con ese modelo:

- tests que setean solo `BACKSCROLL_DATABASE_PATH` pueden leer inputs reales del entorno y colgarse;
- tests de `status`/`validate` que esperan diagnóstico de WAL o ausencia total de side effects ya no describen el contrato público vigente;
- pruebas de unknown flag/command no pueden depender de `stderr` cuando el root usa `SilenceErrors: true`.

## Decisión

1. Hacer herméticas las pruebas de comando que invocan `run` sin política inyectada, usando `setIndexPolicyEnv` o `testEnv`.
2. En pruebas de comando completo, afirmar comportamiento post-startup (índice preparado/sincronizado) en lugar de snapshot-read-only legado.
3. Para unknown flag/command, afirmar sobre `err.Error()` retornado por Cobra.
4. Conservar verificaciones de no mutación estricta solo donde se prueba comportamiento interno/dirigido, no como expectativa global de comandos con startup obligatorio.

## Alternativas descartadas

- **Desactivar startup en tests de comando**: oculta el contrato público y permite regresiones.
- **Mantener expectativas legacy y parchear implementación para evitar sidecars**: contradice la decisión arquitectónica de startup obligatorio y complica paths de compatibilidad.
- **Ignorar solo el timeout de annotate**: deja inestabilidad estructural en la suite.

## Consecuencias

### Positivas

- Se elimina el hang reproducible de `TestAnnotateCommand` por ingestión ambiental.
- `go test ./cmd/backscroll -timeout=180s` completa y ya no falla por fixtures obsoletas de Task 6.
- Las fallas residuales quedan delimitadas a Task 7 (documentación y skill).

### Negativas

- Cambia el significado de varios tests históricos que verificaban “read-only en comando completo”.
- Requiere mantener disciplina de hermeticidad en nuevas pruebas que llamen `run`.

### Riesgo aceptado

Aperturas compatibles de SQLite durante startup pueden crear sidecars (`-shm`/`-wal`) en escenarios específicos; se acepta porque el contrato público prioriza consistencia y frescura del índice sobre inmutabilidad de snapshots en comandos operativos.
