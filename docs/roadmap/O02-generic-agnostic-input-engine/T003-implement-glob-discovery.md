---
estado: Specified
tipo: task
---
# T003: Implement declarative input discovery with globset

**Outcome**: [O02 Generic agnostic input engine](README.md)
**Contribuye a**: CE3, CE4

[[blocked_by:./T002-separate-app-config-from-input-config.md]]

## Preserva

- INV3: No se introducen plugins/scripts ejecutables para el MVP.
  - Verificar: discovery solo usa filesystem/globs en proceso.

## Contexto

Hoy el discovery está hardcodeado: JSON/JSONL, `/subagents/`, `~/.claude/projects`, `.md`, max depth. Debe reemplazarse por reglas genéricas declaradas en TOML.

## Alcance

**In**:
1. Agregar `globset` como dependencia.
2. Implementar discovery con `paths`, `include`, `exclude`, `recursive` si aplica.
3. Soportar archivos directos y directorios.
4. Expandir `~` y rutas relativas de forma documentada.
5. Retornar archivos candidatos ordenados establemente.

**Out**:
- Decoding/parsing de contenido.
- Semánticas específicas como `subagents` en código Rust.

## Estado inicial esperado

- `discover_candidate_files()` en `src/core/sync.rs` filtra extensiones y `/subagents/` en Rust.

## Criterios de Aceptación

- `exclude = ["**/subagents/**"]` reproduce la exclusión Claude sin código específico.
- `include = ["**/*.jsonl"]` cubre JSONL sin extensión hardcodeada.
- Tests cubren include/exclude, archivo directo, directorio y orden determinista.

## Fuente de verdad

- `src/core/sync.rs`
- `Cargo.toml`
- `docs/sync.md`
