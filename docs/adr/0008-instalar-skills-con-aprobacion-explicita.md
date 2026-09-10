---
tipo: adr
estado: proposed
fecha: '2026-09-10'
contexto: 'Los hooks reemplazan la copia Claude y conservan copias obsoletas Agents y OpenCode; issue 62 exige preservar preimagenes y separar mecanismo de despliegue real.'
decision: 'Proponer instalador explicito Python 3 de biblioteca estandar con inventario y aprobacion por digest, enlaces a clon estable, respaldos y recibos restaurables; quitar sustituciones implicitas de skills de ambos hooks.'
alternativas: 'Copias automaticas: mantienen divergencia y borran cambios; enlaces desde el directorio actual: pueden apuntar a una copia desechable; nuevo subcomando Go: amplia el CLI y su politica de arranque para una operacion del repositorio.'
consecuencias: 'Python 3 y Git son requisitos solo del instalador opcional; la actualizacion de un clon estable actualiza los tres destinos; la sustitucion en un hogar real requiere autorizacion independiente.'
pendientes: 'La verificacion de descubrimiento interactivo depende de las interfaces disponibles sin red; no se autoriza despliegue real.'
---
# 0008. Instalar skills con aprobacion explicita

## Contexto

Los hooks reemplazan la copia Claude y conservan copias obsoletas Agents y OpenCode; issue 62 exige preservar preimagenes y separar mecanismo de despliegue real.

## Decisión

Proponer instalador explicito Python 3 de biblioteca estandar con inventario y aprobacion por digest, enlaces a clon estable, respaldos y recibos restaurables; quitar sustituciones implicitas de skills de ambos hooks.

## Alternativas descartadas

Copias automaticas: mantienen divergencia y borran cambios; enlaces desde el directorio actual: pueden apuntar a una copia desechable; nuevo subcomando Go: amplia el CLI y su politica de arranque para una operacion del repositorio.

## Consecuencias

Python 3 y Git son requisitos solo del instalador opcional; la actualizacion de un clon estable actualiza los tres destinos; la sustitucion en un hogar real requiere autorizacion independiente.

## Pendientes

La verificacion de descubrimiento interactivo depende de las interfaces disponibles sin red; no se autoriza despliegue real.
