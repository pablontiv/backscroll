---
id: T030
tipo: task
estado: Pending
titulo: Tests de integración del pipeline
outcome: O08
dependencias: [T029]
---

# T030 — Tests de integración del pipeline

Tests de integración end-to-end que cubren Claude JSONL, Pi JSONL y OpenCode SQLite
pasando por el pipeline genérico completo (Discover → Hash → Parse → SyncFiles).

## Alcance

En `internal/readers/integration_test.go` o `cmd/backscroll/main_test.go`:

- Test Claude fixture: `backscroll sync --path tests/fixtures/claude_session.jsonl`
  produce los mismos `ParsedFile` que el sync legacy
- Test Pi fixture: `backscroll sync` con `pi.inputs.toml` + fixture Pi
- Test OpenCode fixture: crear un `.opencode/opencode.db` de test con sesiones de prueba;
  `backscroll sync` con `opencode.inputs.toml` indexa correctamente
- Test de dedup: sync dos veces → segunda vez 0 archivos nuevos
- Test de predicados: fixture con registros que deben ser filtrados por predicado
- Test de text transforms: fixture con contenido que debe ser transformado

## Criterios de aceptación

- Todos los tests pasan con `go test -race ./...`
- Coverage ≥85% mantenida (verificar con `just coverage`)
- Fixture OpenCode DB creado programáticamente en el test (no depende de DB real)
- Tests herméticamente aislados (no leen `~/.claude/` ni `~/.config/backscroll/`)

## Notas

- Usar `t.TempDir()` para directorios temporales
- Fixture OpenCode DB: crear con `modernc.org/sqlite` + schema mínimo de OpenCode
