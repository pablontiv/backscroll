---
estado: Completed
tipo: task
---
# T004: Parsers de sources y tagging

**Outcome**: [Port a Go](README.md)

## Contexto

Tres módulos de lógica pura sin dependencias de SQLite: parsers de plans (markdown via goldmark), parsers de sources externos (KE, decision, memory, rule, spec, backlog) y heurísticas de auto-tagging. Equivalentes a `core/plans.rs`, `core/sources.rs` y `core/tagging.rs`.

## Alcance

**In**:
1. `internal/plans` — parser de `~/.claude/plans/*.md` usando goldmark; split por `##` headers, cada sección como item indexable con `source='plan'`.
2. `internal/sources` — parsers para los 6 tipos de sources externos: whole-document y sectioned by `##` headers. `SourceRegistry` con config desde `[sources]` de backscroll.toml.
3. `internal/tagging` — heurísticas regexp para categorías: debugging, refactoring, feature, testing, docs, config. `regexp.MustCompile` como equivalente a `LazyLock`.
4. Tests con los fixtures existentes: `tests/fixtures/ke-001.md`, `decisions-test-*.md`, `memory-test.md`, `rule-test.md`, `spec-test.md`, `backlog-test.md`.

**Out**:
1. Escritura a SQLite (va en T005).

## Criterios de Aceptación

- Plans parser produce el mismo número de secciones para un fixture dado que la versión Rust.
- Source parsers cubren los 6 tipos; tests con cada fixture pasan.
- Tagging detecta correctamente las 6 categorías en sesiones de ejemplo.
- `go test ./internal/plans/... ./internal/sources/... ./internal/tagging/...` pasa.
