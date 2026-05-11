# Intención: Backscroll agnóstico de CLI agentic

## 1) Propósito

Backscroll interpreta sesiones y documentos mediante **definiciones externas en TOML** para poder agregar soporte de nuevos agentes sin recompilar. La ubicación canónica runtime es global y user-scoped:

```text
<config_dir>/backscroll/inputs/*.inputs.toml
```

`<config_dir>` es el directorio de configuración del sistema operativo, o `BACKSCROLL_CONFIG_DIR` cuando se define.

## 2) Alcance MVP vigente

- Inputs externos para sesiones con `source = "session"`.
- Preset shipped para Claude en `inputs/claude.inputs.toml`, instalable en el config dir del usuario; otros agentes se agregan como manifests user-scoped.
- Manifests data-only: discovery, decode, selectors, predicates, mapping y normalización de texto.
- App config (`backscroll.toml`) separada de input config.
- **Regla absoluta:** no hay soporte de adapters ejecutables, scripts externos ni JMESPath en este MVP.

## 3) Principios

1. **Sin recompilar para agregar inputs:** agregar/modificar TOML instalado en el config dir global es suficiente.
2. **Agnóstico por diseño:** Backscroll no debe depender de semántica hardcodeada de cada CLI.
3. **Config separada:** `backscroll.toml` configura la app; `*.inputs.toml` configura ingesta.
4. **Contrato interno estable:** normalizar a `ParsedFile` y `ParsedMessage`.
5. **Incremental y riesgo acotado:** conservar deduplicación por hash/path y salida de búsqueda estable.

## 4) Invariantes a preservar

### A. Usuario/CLI

1. Los comandos públicos (`search`, `resume`, `list`, `topics`, `insights`, `export`, `status`, `sync`) siguen disponibles; path lookup se hace con `search --source-path`.
2. El autosync previo a comandos se conserva.
3. `source = "session"` permanece como valor estable para conversaciones de Claude/Pi.
4. `backscroll inputs validate/list/test` son el punto de diagnóstico para manifests.

### B. Configuración

5. `database_path` por defecto sigue en `~/.backscroll.db`.
6. El loader canónico lee solo manifests globales bajo `<config_dir>/backscroll/inputs/`.
7. `BACKSCROLL_CONFIG_DIR` permite pruebas e instalaciones personalizadas.
8. Presets instalados no deben sobrescribir manifests existentes por defecto.

### C. Ingestión y datos

9. `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
10. Deduplicación incremental por hash/path sigue vigente.
11. `uuid` y `source_path` siguen siendo identidad operativa.
12. Fallos puntuales de registros no deben romper toda la sync cuando el manifest en sí es válido.

### D. Calidad y pruebas

13. Tests de loader global prueban que manifests locales no se leen como fuente canónica.
14. Tests de CLI usan `BACKSCROLL_CONFIG_DIR` para fixtures.
15. Docs y skill se mantienen alineadas con la ruta global de inputs.

## 5) No-goals del MVP

- No introducir adapters ejecutables ni scripts de integración.
- No introducir JMESPath ni plugins.
- No mezclar rutas de ingesta en `backscroll.toml`.
- No cambiar esquema SQLite salvo tareas explícitas.
- No rediseñar el CLI ni la UX de búsqueda.

## 6) Criterios de éxito

- Backscroll indexa inputs definidos por TOML global sin recompilar.
- Claude y Pi están cubiertos por presets shipped instalables en user scope.
- App config e input config están claramente separadas.
- Validación, dry-run, sync y autosync usan el mismo modelo de input manifests; lectura pública usa resultados indexados vía SQLite.
