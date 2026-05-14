---
id: T029
tipo: task
estado: Completed
titulo: Refactor sync command para usar pipeline genérico
outcome: O08
dependencias: [T024, T026, T027, T028, T019]
---

# T029 — Refactor `sync` command para usar pipeline genérico

Conectar el comando `backscroll sync` al registry de readers. En lugar de llamar
directamente a `WalkSessionDirs()` + `ParseSessions()`, iterar sobre los inputs
activos y despachar cada uno al reader correspondiente.

## Alcance

En `cmd/backscroll/sync.go` y `internal/sync/sync.go`:

```
1. Cargar inputs activos (LoadInputs() o compat manifest de session_dirs)
2. Para cada InputDefinition:
   a. Resolver el reader por decode.format ("jsonl" → JsonlReader, "opencode" → OpenCodeReader)
   b. Discover() → lista de sesiones
   c. Para cada sesión: Hash() → dedup check → Parse() si cambió → SyncFiles()
3. Reportar progreso y estadísticas
```

- Mantener `--path` flag funcional (override del discover para un path específico)
- Mantener `--include-agents` flag
- Mantener `--no-plans` flag
- `--dry-run` flag (si no existe, agregarlo): muestra qué se sincronizaría sin escribir

## Criterios de aceptación

- `backscroll sync` con configuración actual produce el mismo resultado que antes
- `backscroll sync` con `opencode.inputs.toml` activo indexa sesiones OpenCode
- `backscroll sync --path <file>` sigue funcionando para un archivo específico
- Todos los tests existentes de sync pasan sin modificación
- `go test ./...` pasa con coverage ≥85%

## Notas

- `internal/sync/sync.go` puede reducirse o convertirse en helpers internos;
  el flujo de control principal pasa a `cmd/backscroll/sync.go`
