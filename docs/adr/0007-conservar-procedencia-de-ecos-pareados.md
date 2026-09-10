---
tipo: adr
estado: proposed
fecha: '2026-09-10'
contexto: 'La salida real de búsquedas pareadas desplaza el resultado histórico del puesto cuatro al siete; el texto almacenado perdió el vínculo de llamada.'
decision: 'Persistir evidencia positiva del lector en search_echo mediante v15; filtrar solo ecos directos en ambas listas de candidatos sin afectar búsquedas explícitas de herramientas.'
alternativas: 'Inferir por forma o adyacencia del resultado: descartado por falsos positivos; excluir sesiones completas: fuera del alcance; subir la época general de extracción: provoca reprocesamiento ajeno innecesario.'
consecuencias: 'Reprocesar fuentes disponibles mediante la cola incremental y actualizar solo metadatos sin reemplazar identidades; conservar evidencia al recuperar índices; los resultados antiguos con fuentes expiradas permanecen consultables.'
pendientes: 'Revisión independiente del nuevo candidato antes de aprobar la integración.'
---
# 0007. Conservar procedencia de ecos pareados

## Contexto

La salida real de búsquedas pareadas desplaza el resultado histórico del puesto cuatro al siete; el texto almacenado perdió el vínculo de llamada.

## Decisión

Persistir evidencia positiva del lector en search_echo mediante v15; filtrar solo ecos directos en ambas listas de candidatos sin afectar búsquedas explícitas de herramientas.

## Alternativas descartadas

Inferir por forma o adyacencia del resultado: descartado por falsos positivos; excluir sesiones completas: fuera del alcance; subir la época general de extracción: provoca reprocesamiento ajeno innecesario.

## Consecuencias

Reprocesar fuentes disponibles mediante la cola incremental y actualizar solo metadatos sin reemplazar identidades; conservar evidencia al recuperar índices; los resultados antiguos con fuentes expiradas permanecen consultables.

## Pendientes

Revisión independiente del nuevo candidato antes de aprobar la integración.
