---
tipo: outcome
estado: Pending
---
# Workspace bucketing por cwd

El bucketing de sesiones a los workspaces de `projects.toml` no funciona para ninguna fuente file-based (Claude/Pi/OpenCode): el `cwd` de la sesión nunca llega a `projects.Identify`. La indexación pasa el path del archivo de sesión (`ref`) en vez del cwd, y el pipeline declarativo ni siquiera extrae `Map.Project` (`$.cwd`). Resultado: todas las sesiones resuelven a `unknown` y buckets como `pinata` quedan vacíos.

Este outcome plumbea el `cwd` de la sesión de punta a punta (parser → `ParsedFile` → sync) y lo alimenta a `Identify`, de modo que las sesiones resuelvan a su proyecto vía roots de `projects.toml` y/o hints `.backscroll/project.toml`.
