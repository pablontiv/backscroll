---
id: T016
tipo: task
estado: Completed
titulo: Glob discovery declarativo
outcome: O07
dependencias: [T014, T015]
---

# T016 — Glob discovery declarativo

Implementar el motor de discovery de archivos basado en los patrones `include`/`exclude`
del `DiscoverConfig`. Reemplaza el discovery hardcodeado de `.jsonl` + filtro `/subagents/`
en `internal/sync/sync.go`.

## Alcance

En `internal/input_config/discover.go`:

```go
// DiscoverFiles retorna los paths de archivos que coinciden con los patrones del config.
// Expande globs, aplica exclude patterns, respeta follow_symlinks.
func DiscoverFiles(cfg DiscoverConfig, baseDir string) ([]string, error)
```

- Expandir glob patterns en `include` (e.g., `~/.claude/projects/**/*.jsonl`)
- Filtrar paths que coinciden con cualquier patrón en `exclude`
- Seguir symlinks si `follow_symlinks = true` (cuidado con loops)
- Normalizar paths a absolutos

## Criterios de aceptación

- Con `include = ["**/*.jsonl"]` + `exclude = ["**/subagents/**"]` descubre los mismos archivos que el sync actual en `~/.claude/projects/`
- Test table-driven con fixtures temporales (archivos y symlinks)
- `follow_symlinks = false` no cruza symlinks
- No panic con patrones inválidos (retornar error descriptivo)

## Referencias

- `DiscoverConfig` / `discover()` en `src/input_config.rs` (v0 branch)
- `internal/sync/sync.go:WalkSessionDirs()` — comportamiento actual a replicar
