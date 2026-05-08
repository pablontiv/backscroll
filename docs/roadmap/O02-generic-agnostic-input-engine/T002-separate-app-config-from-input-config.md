---
estado: Specified
tipo: task
---
# T002: Separate app config from input config

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE2

[[blocked_by:./T001-define-generic-input-contract.md]]

## Preserva

- INV1: `source = "session"` permanece como categoría semántica estable para conversaciones.
  - Verificar: input config conserva `source`, app config no lo redefine.
- INV2: `ParsedFile` y `ParsedMessage` siguen siendo la frontera interna de ingestión.
  - Verificar: solo cambia carga/configuración previa a parseo.

## Contexto

`backscroll.toml` debe ser configuración de la app; `*.inputs.toml` debe ser configuración de quien llama/genera inputs. Hoy `Config` mezcla `session_dirs`, `sources` y `session_inputs` con `database_path`/embedding.

## Alcance

**In**:
1. Crear modelo de input config separado de `Config`.
2. Cargar archivos `*.inputs.toml` desde ubicación definida por T001.
3. Remover `session_dirs` del camino principal si no hay compatibilidad legacy requerida.
4. Mantener `database_path`, embedding y opciones globales en app config.
5. Actualizar docs para describir la separación.

**Out**:
- Engine JSONL.
- Migrar plans/external sources completos.

## Estado inicial esperado

- `src/config.rs` contiene `Config { database_path, session_dirs, embedding, sources, session_inputs }`.
- `Config::collect_backscroll_inputs()` carga `backscroll.inputs.toml` y `backscroll.inputs.d/*.toml`.

## Criterios de Aceptación

- `backscroll.toml` ya no es fuente primaria de rutas de ingesta.
- Los inputs se cargan desde `*.inputs.toml` y fallan con error claro si son inválidos.
- No hay `session_dirs` alimentando silenciosamente parser Claude en el flujo canónico.
- Tests de config cubren app config e input config por separado.

## Fuente de verdad

- `src/config.rs`
- `src/main.rs`
- `docs/configuration.md`
- `backscroll.toml.example`
