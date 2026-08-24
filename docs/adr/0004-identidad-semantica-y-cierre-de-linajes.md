---
tipo: adr
estado: accepted
fecha: "2026-08-24"
contexto: Backscroll ha rechazado repetidamente bases SQLite sanas porque la identidad de compatibilidad mezcla semántica actual, formato textual del DDL y checksums históricos; V14 volvió a bloquear dos linajes V13 reconocidos y la prueba de migración los ocultó mediante skips.
decision: Identificar los esquemas mediante una forma semántica híbrida, conservar el ledger como proveniencia separada y exigir que cada fixture histórica reconocida desde V1 migre hasta head mediante la ruta productiva sin exclusiones; recover será una ruta de remediación que no depende de la preparación ordinaria.
consecuencias: Las diferencias cosméticas y las historias equivalentes dejarán de multiplicar linajes, toda migración futura probará automáticamente el corpus histórico completo y los esquemas semánticamente desconocidos seguirán fallando de forma cerrada; aumentará la responsabilidad del canonicalizador y de las pruebas de no equivalencia.
---

# Adoptar identidad semántica y cierre completo de linajes

## Contexto

La compatibilidad de índices se introdujo para reconocer la forma real de una base SQLite y no confiar solamente en su versión de migración. La implementación actual firma metadatos estructurados, texto DDL normalizado y filas de `schema_migrations`, y consulta un catálogo hermético de releases y formas observadas.

Este modelo produjo bloqueos recurrentes. Los issues #41 y #52 incorporaron formas V13 faltantes y corrigieron diferencias de whitespace, un catálogo duplicado y defectos léxicos. Sin embargo, V14 preservó diferencias textuales heredadas por tablas construidas mediante `ALTER TABLE`. Dos formas V13 ya reconocidas produjeron firmas V14 desconocidas; la verificación anterior al commit abortó y todos los comandos operacionales quedaron bloqueados.

La especificación sistémica previa exigía que cada linaje publicado alcanzara head sin pérdida y sin pruebas omitidas. La implementación contradijo esa garantía al excluir expresamente las dos fixtures ALTER-built de la prueba de migración. El defecto estaba presente en el corpus, pero la suite fue configurada para no ejecutarlo.

La causa no se limita a V13. Cualquier forma histórica reconocida desde V1 puede conservar diferencias de construcción a través de migraciones posteriores. Añadir firmas V14 resolvería el bloqueo inmediato, pero permitiría que una migración futura repitiera la multiplicación.

También se verificó que `recover --from … --dry-run` no remedia este fallo: la política de startup intenta primero la misma preparación incompatible y vuelve a emitir `migration_failed`.

## Decisión

La identidad de compatibilidad se dividirá en dos conceptos:

1. **Forma semántica actual.** Será la clave para seleccionar un plan. Combinará metadatos estructurados de SQLite con SQL auxiliar canonicalizado para constraints, expresiones, triggers, índices parciales y tablas virtuales que PRAGMA no describe completamente.
2. **Proveniencia de migración.** Las filas, nombres y checksums de `schema_migrations` seguirán inspeccionándose, preservándose y documentándose, pero no crearán identidades distintas cuando la versión aplicada y la forma semántica sean equivalentes.

El canonicalizador eliminará comentarios y diferencias irrelevantes de whitespace o puntuación fuera de literales e identificadores citados. Preservará strings, identificadores citados y diferencias conductuales. No incorporará un parser SQL completo.

El catálogo continuará siendo la fuente hermética de releases y formas observadas. Varias fixtures físicas podrán compartir firma semántica, pero cada fixture permanecerá como entrada independiente de prueba. Una colisión será válida únicamente si coincide la versión aplicada, la evidencia estructural, la semántica auxiliar y el plan restante.

La prueba de cierre se derivará automáticamente de todas las fixtures. Para cada forma reconocida desde V1 ejecutará `OpenCompatible`, aplicará todas las migraciones reales hasta head y verificará forma final, filas, UUIDs, satélites, FTS, ledger y snapshots aplicables. No tendrá allowlist manual ni rama de `t.Skip`. Añadir una migración futura extenderá automáticamente todas las trayectorias históricas al nuevo head.

`recover` se clasificará como remediación. Conservará el lock exclusivo, pero omitirá `OpenCompatible` y el sync previo al handler. Su planner abrirá los inputs mediante rutas read-only y, en apply, conservará el lock durante reemplazo, verificación y post-install sync. `--dry-run` no modificará datos de entrada.

Los esquemas cuya forma semántica no figure en el catálogo seguirán devolviendo `unsupported_lineage`. No se añadirán flags de bypass, catálogos paralelos, reparación automática ni excepciones runtime para V14.

## Alternativas descartadas

### Registrar las firmas V14 faltantes

Desbloquea los casos actuales con poco código, pero conserva la multiplicación de identidades por historia y formato.

### Ajustar solamente whitespace y puntuación

Corrige el mecanismo inmediato, pero mantiene el ledger histórico y otras diferencias textuales dentro de la identidad actual.

### Construir un parser DDL completo

Podría producir una representación AST más profunda, pero añade un subsistema complejo para dialecto, triggers, virtual tables y extensiones futuras. El modelo híbrido prioriza metadatos de SQLite.

### Confiar solamente en versión y checksum

Versiones iguales ya han ocultado formas distintas, mientras checksums distintos pueden terminar en la misma forma actual. Esta alternativa confunde proveniencia con compatibilidad.

### Mantener recover en la preparación ordinaria

Obliga al comando reparador a superar primero la condición que debe reparar y produce una continuación operacionalmente circular.

## Consecuencias

### Positivas

- Las diferencias cosméticas y las rutas equivalentes dejan de multiplicar identidades.
- Cada release y forma observada permanece representada por una fixture verificable.
- Toda migración futura prueba automáticamente cada trayectoria histórica desde V1.
- La suite no puede declarar soportada una forma y excluirla simultáneamente.
- Los usuarios ALTER-built alcanzarán head sin editar `sqlite_master`.
- `recover --dry-run` podrá alcanzar su planner aunque la preparación ordinaria falle.

### Negativas

- El canonicalizador auxiliar será una frontera de seguridad con pruebas exigentes.
- Regenerar el catálogo cambiará firmas aunque los bytes de fixtures no cambien.
- La matriz histórica aumentará el tiempo de pruebas de storage.
- Separar proveniencia e identidad exige aclarar APIs cuyo campo se denomina `Signature`.

### Riesgo aceptado

La canonicalización será conservadora: puede mantener separadas algunas formas equivalentes no demostradas por fixtures, pero no debe fusionar formas con comportamiento distinto. Se aceptan falsos negativos documentables para evitar falsos positivos inseguros.
