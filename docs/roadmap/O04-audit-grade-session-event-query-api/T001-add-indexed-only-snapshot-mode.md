---
estado: Specified
tipo: task
---
# T001: Add indexed-only snapshot mode

**Outcome**: [Audit-grade session event query API](README.md)

## Preserva

- Existing default command behavior remains unchanged unless a user opts into the new flag.
- The new mode never mutates the Backscroll database or input manifests.

## Contexto

Pinata session-audit promises not to run backscroll sync, but current read commands may auto-sync. Audit consumers need a stable snapshot mode before they can rely on Backscroll as their corpus boundary.

## Alcance

**In**:
1. Add an indexed-only/no-sync flag to relevant read commands, starting with list.
2. Ensure the command path bypasses autosync and reads only the existing SQLite/index state.
3. Add tests that fail if the indexed-only path invokes sync or mutates the index.

**Out**:
1. Changing default autosync behavior for existing users.
2. Adding normalized event storage or event query commands.

## Estado inicial esperado

Backscroll read commands can discover/index sessions, but deterministic downstream tools lack a documented way to request a no-mutation indexed snapshot.

## Criterios de Aceptación

- `backscroll list --indexed-only --json` or the chosen equivalent succeeds using only existing index data.
- The indexed-only mode returns a clear diagnostic when no usable index exists.
- Automated tests prove the indexed-only path does not call sync/discovery mutation code.

## Fuente de verdad

- src/main.rs
- src/core/sync.rs
- tests/cli.rs
- docs/sync.md
