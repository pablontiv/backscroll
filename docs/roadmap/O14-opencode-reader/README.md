---
tipo: outcome
---
# O14: OpenCode Reader

Backscroll puede indexar conversaciones de OpenCode (anomalyco/opencode v1.14+) — el lector lee desde la DB SQLite global y el preset `opencode.inputs.toml` permite activarlo con un solo `cp` + `backscroll sync`.

El lector existente (`OpenCodeReader`) fue escrito para el schema Go de `opencode-ai/opencode` (tabla `messages` con columnas `role`/`parts`). El binario instalado (`anomalyco/opencode`, TypeScript/Bun) usa un schema diferente: tablas `message` y `part` separadas donde el rol y el contenido viven en blobs JSON `data`. Sin la corrección el reader falla en tiempo de ejecución con cualquier DB que tenga datos.
