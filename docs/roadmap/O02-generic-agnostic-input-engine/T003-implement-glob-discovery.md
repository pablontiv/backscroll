---
estado: Completed
tipo: task
---
# T003: Implement declarative input discovery with globset

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE3, CE4

[[blocked_by:./T002-separate-app-config-from-input-config.md]]

## Preserva

- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: discovery solo usa filesystem/globs en proceso, sin adapters externos.

## Contexto

Hoy el discovery está hardcodeado: extensiones, `/subagents/`, `~/.claude/projects`, `.md`, max depth. En O02 esas reglas deben venir de la sección `[inputs.discover]` del contrato final:

```toml
[inputs.discover]
roots = ["~/.claude/projects"]
include = ["**/*.jsonl"]
exclude = ["**/subagents/**"]
follow_symlinks = false
```

El core aplica reglas genéricas de roots/include/exclude; no interpreta semánticas como `subagents`.

## Alcance

**In**:
1. Agregar `globset` como dependencia para matching de `include`/`exclude`.
2. Implementar discovery con `discover.roots`, `discover.include`, `discover.exclude` y `discover.follow_symlinks` según `docs/input-contract.md`.
3. Soportar roots que apunten a archivos directos o directorios.
4. Expandir `~` y resolver rutas relativas de forma documentada y testeada.
5. Retornar archivos candidatos ordenados establemente.
6. Tratar manifests activos con roots/globs inválidos como error claro en flujos manifest-driven.

**Out**:
- Decoding/parsing de contenido.
- Semánticas específicas como `subagents` en código Rust.
- Reintroducir `--path`/`session_dirs` como discovery canónico.

## Estado inicial esperado

- `discover_candidate_files()` en `src/core/sync.rs` filtra extensiones y `/subagents/` en Rust.
- T002 separó app config de manifests de input.

## Criterios de Aceptación

- `exclude = ["**/subagents/**"]` reproduce la exclusión Claude sin código específico.
- `include = ["**/*.jsonl"]` cubre JSONL sin extensión hardcodeada.
- `roots` soporta archivo directo y directorio.
- `follow_symlinks` tiene default `false` y comportamiento documentado.
- Tests cubren include/exclude, archivo directo, directorio, symlinks si aplica y orden determinista.

## Fuente de verdad

- `docs/input-contract.md`
- `src/core/sync.rs`
- `Cargo.toml`
- `docs/sync.md`
