---
tipo: adr
estado: accepted
fecha: "2026-08-24"
contexto: La firma semántica dejó de incluir las filas históricas de schema_migrations. Como consecuencia, varias bases pueden compartir la misma firma de forma actual aunque tengan distinta versión aplicada y, por lo tanto, distinto plan de migración restante.
decision: Consultar el catálogo de linajes mediante la forma completa compuesta por AppliedVersion y Signature. La proveniencia del ledger se inspecciona y valida de manera privada, pero no participa en la firma semántica ni se expone en SchemaShape.
consecuencias: El catálogo puede representar firmas compartidas sin ambigüedad, recovery rechaza una firma conocida con versión desconocida y las APIs de solo firma se eliminan; los llamadores deben transportar SchemaShape completo en lugar de strings de firma aislados.
---

# Consultar catálogo de linajes por forma completa

## Contexto

La separación entre identidad semántica y proveniencia de migración evita que dos bases con la misma forma actual queden en linajes diferentes solo por checksums, nombres o relojes históricos del ledger. Esa separación también permite que dos versiones aplicadas compartan una misma firma semántica.

Con una consulta basada únicamente en `Signature`, el catálogo tendría que escoger un único ganador para firmas compartidas. Ese ganador podría tener un `AppliedVersion` distinto al observado y producir pasos de migración incorrectos. Recovery presentaba el mismo riesgo: podía aceptar una firma conocida aunque la versión aplicada no perteneciera al catálogo.

## Decisión

La identidad operacional de catálogo será `SchemaShape{AppliedVersion, Signature}`. Internamente se representa con una clave compuesta `lineageKey` y se exponen las operaciones `ByShape`, `IsKnownShape` y `CurrentShape`.

Las filas de `schema_migrations` se cargan como proveniencia privada, se validan en orden y determinan `AppliedVersion`, pero no se agregan a los registros semánticos que producen `Signature`. Recovery valida la forma completa recibida desde el plan de compatibilidad antes de leer registros recuperables.

## Alternativas descartadas

### Mantener consulta solo por firma

Se descartó porque una firma compartida entre versiones aplicadas distintas vuelve ambigua la selección de `remainingSteps` y puede aceptar bases no catalogadas.

### Incluir la versión aplicada dentro de la firma

Se descartó porque volvería a mezclar identidad semántica con estado de migración. La firma debe describir la forma actual; la versión aplicada debe participar en la identidad de catálogo como dimensión separada.

### Exponer proveniencia completa en `SchemaShape`

Se descartó porque los nombres y checksums del ledger son evidencia histórica, no contrato público de compatibilidad ni entrada necesaria para planes de migración.

## Consecuencias

- Varias fixtures físicas pueden compartir firma sin colisionar en el catálogo.
- Los llamadores deben conservar y pasar `SchemaShape` completo.
- `BySignature`, `IsKnownSignature` y `CurrentSignature` dejan de existir.
- Los diagnósticos de linaje no soportado deben incluir versión y firma para explicar la identidad rechazada.
