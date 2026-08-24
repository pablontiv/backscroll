---
tipo: adr
estado: accepted
fecha: "2026-08-24"
contexto: El catálogo puede contener varias firmas semánticas legítimas para versiones históricas, como V3 y V5, mientras que la cabeza semántica puede provenir de una fixture no manifestada con AppliedVersion mayor que LatestGoRelease.
decision: Seleccionar CurrentShape mediante la máxima AppliedVersion inventariada y validar ambigüedad solo entre las formas que comparten esa versión máxima. LatestGoRelease conserva su rol de validación de inventario, pero no decide la cabeza semántica.
consecuencias: Las firmas históricas múltiples dejan de producir fallos dependientes del orden de iteración de mapas; una competencia real en la versión máxima sigue fallando de forma cerrada; las pruebas de cierre comparan cero pasos contra la forma semántica corriente real.
---

# Seleccionar cabeza semántica por versión máxima

## Contexto

El catálogo de compatibilidad representa evidencias físicas de releases publicadas y fixtures locales no manifestadas. Algunas versiones históricas tienen más de una firma semántica legítima porque las bases observadas pueden diferir en columnas fantasma o metadatos de origen y aun así conservar planes de migración válidos.

La selección previa de cabeza dependía del release Go más reciente. Durante la transición a identidad semántica compuesta, una implementación parcial intentó calcular la versión máxima y validar firmas en una sola pasada sobre un `map`. Ese enfoque podía tratar temporalmente firmas V3 o V5 como cabeza corriente antes de observar V14, lo que introducía fallos intermitentes por orden de iteración.

## Decisión

`Catalog.attachLineages` seleccionará `CurrentShape()` en dos pasos:

1. calcular la máxima `AppliedVersion` entre todos los linajes inventariados;
2. revisar únicamente los linajes con esa versión máxima.

Si todos los linajes de la versión máxima tienen la misma firma semántica, esa forma se considera cabeza corriente. Si existen firmas distintas en la versión máxima, el catálogo falla cerrado con error de ambigüedad.

`LatestGoRelease` permanece como validación de inventario en la carga del manifiesto: debe existir, cumplir el piso de versión esperado y conservar fixtures verificables. No participa en la selección de cabeza semántica.

## Alternativas descartadas

### Usar `LatestGoRelease` como fuente de verdad de cabeza

Se descartó porque la cabeza semántica puede estar representada por una fixture local no manifestada con mayor `AppliedVersion`. En ese caso, usar el release Go más reciente degradaría la cabeza a una versión anterior.

### Aceptar cualquier firma cuando no quedan pasos

Se descartó porque relajaría la frontera de compatibilidad. Una forma corriente ambigua en la máxima versión debe detener la carga para evitar aceptar estados no catalogados.

### Agregar aliases o fixtures `*-current.sql`

Se descartó porque las fixtures ALTER-built ya son estructuralmente equivalentes a la cabeza canónica salvo formato no semántico. Crear aliases encubriría el problema de selección en lugar de corregir la regla del catálogo.

## Consecuencias

- Las firmas múltiples en versiones históricas no-cabeza son compatibles con el catálogo.
- La selección de cabeza ya no depende del orden no determinista de iteración de mapas en Go.
- Una divergencia real entre firmas de la máxima `AppliedVersion` sigue siendo un error fatal de catálogo.
- Las pruebas de cierre pueden exigir simultáneamente cero pasos restantes y `plan.From == catalog.CurrentShape()` para cada fixture física.
