---
tipo: adr
estado: accepted
fecha: 2026-08-20
contexto: El port Go reintrodujo el comando público read y varias rutas indexed-only que permiten consultar archivos o snapshots sin pasar por la ingesta y el índice perenne definidos por el North Star.
decision: Ejecutar un sync incremental central antes de toda operación, retirar read e indexed-only y permitir únicamente recover como continuación controlada después de un intento de sync fallido.
consecuencias: SQLite vuelve a ser la única fuente pública de consulta; el CLI pierde superficies incompatibles y todos los comandos operativos asumen un índice recién sincronizado.
---

# Exigir sync y consulta desde SQLite

## Contexto

Backscroll define los archivos de sesiones, planes y documentos como inputs transitorios. SQLite es el registro episódico perenne: conserva información cuando los archivos expiran y debe suministrar toda consulta visible al usuario.

El comando `backscroll read` viola esa frontera porque abre un archivo físico mediante `internal/reader` sin ingerirlo ni consultar SQLite. El flag `--indexed-only` crea una segunda excepción al omitir discovery y sync para consultar una snapshot existente. Además, `prepareIndex(..., autoSync bool)` distribuye la decisión de frescura entre handlers.

La tarea histórica `docs/roadmap/T001-remove-public-read-command.md` ya había identificado y eliminado `read` en `f9c37eb`. El port Go lo reintrodujo en `104c81e` al reconstruir el inventario anterior de comandos. PR #43 alineó las guías con el árbol Cobra resultante, mostrando que validar documentación contra Cobra no basta cuando Cobra contradice la arquitectura.

## Decisión

Toda operación pública ejecutará una política central de arranque desde `PersistentPreRunE`:

1. cargar configuración;
2. rechazar fuentes legacy;
3. validar todos los manifests activos;
4. preparar un índice compatible;
5. ejecutar exactamente un intento de sync incremental de arranque;
6. ejecutar el handler solicitado solamente después del éxito.

Se retiran sin periodo de deprecación:

- el comando público `backscroll read`;
- el paquete `internal/reader` cuando quede sin consumidores;
- todos los flags y caminos `--indexed-only`;
- la decisión `autoSync` distribuida entre comandos.

`recover` es la única continuación permitida después de un intento de sync fallido, porque su función es reparar el estado que impidió completar el arranque. Tras instalar y verificar la base canónica, ejecuta un sync post-instalación antes de informar éxito; ese segundo intento opera sobre la base recuperada, no repite el arranque contra la base fallida. No puede devolver resultados cached. Help y version permanecen libres de efectos laterales porque no ejecutan un cuerpo operativo.

Los planes y documentos Markdown se ingieren mediante manifests y se consultan desde SQLite. `search --source-path` permanece como búsqueda DB-backed por ruta.

Esta decisión limita ADR 0001: Cobra sigue siendo la fuente de verdad de la sintaxis publicada, pero no de los invariantes arquitectónicos. Tests dedicados deben impedir que Cobra vuelva a registrar superficies prohibidas.

Quedan supersedidas las secciones de diseños posteriores que preservaron `read` directo o presentaron `--indexed-only` como contrato público vigente. Los documentos históricos permanecen intactos como evidencia de contexto.

## Alternativas descartadas

- **Forzar sync en cada handler:** conserva una política distribuida que un comando nuevo puede omitir.
- **Sincronizar antes de parsear Cobra:** produciría efectos laterales para help, version y argumentos inválidos, y dificultaría la continuación de recovery.
- **Mantener read como diagnóstico:** conserva una segunda fuente pública de verdad y repite la ambigüedad que originó la regresión.
- **Conservar indexed-only para auditorías:** contradice la frescura obligatoria; una futura API de snapshot necesita un contrato separado y versionado.
- **Añadir un daemon:** no es necesario; el hash incremental antes de cada operación satisface la garantía aprobada.

## Consecuencias

### Positivas

- SQLite es la única superficie pública de consulta.
- Todos los comandos nuevos heredan sync sin configuración adicional.
- Los fallos de ingesta abortan sin servir filas stale.
- Los tests pueden validar tanto documentación→CLI como CLI→arquitectura.

### Negativas

- `read` y `--indexed-only` se eliminan como breaking changes.
- `config`, `status` y `validate` pasan a ejecutar trabajo de sync antes de responder.
- Consumidores que dependían de snapshots deben migrar.
- Numerosos tests herméticos requieren manifests en lugar del bypass indexed-only.

### Riesgo aceptado

Cada invocación paga discovery y hashing incremental. Los archivos sin cambios se omiten; se acepta ese costo para preservar la consistencia de la memoria episódica.
