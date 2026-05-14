---
estado: Completed
tipo: task
---
# T012: Cerrar PRs huérfanos de deps Rust y limpiar dependabot

**Contribuye a**: Eliminar PRs stale que nunca podrán mergearse (Cargo.toml ausente en main)

## Preserva

- INV1: `main` no tiene Cargo.toml ni Cargo.lock
  - Verificar: `gh api repos/pablontiv/backscroll/contents/Cargo.toml` retorna 404

## Contexto

PRs #12, #13, #15, #16 son bumps de deps Rust (ndarray, sha2, insta, sqlite-vec) creados
por Dependabot. Cargo.toml y Cargo.lock no existen en `main` — el proyecto migró a Go.
Estos PRs nunca podrán mergearse. Además, dependabot sigue configurado para el ecosistema
Rust y generará nuevos PRs sin corregir la config.

## Alcance

**In**:
1. Cerrar PR #12, #13, #15, #16 con mensaje: "Cargo.toml/Cargo.lock no existen en main — el proyecto migró a Go. Cerrando PR stale. Actualizar dependabot.yml para remover el ecosistema cargo."
2. Editar `.github/dependabot.yml` para remover entradas del ecosistema `cargo`
3. Commit del cambio en dependabot.yml

**Out**:
- No restaurar Cargo.toml ni código Rust

## Estado inicial esperado

- 4 PRs abiertos (#12, #13, #15, #16) para deps Rust
- Cargo.toml/Cargo.lock ausentes en main

## Criterios de Aceptación

- `gh pr list --repo pablontiv/backscroll --state open` retorna 0 PRs
- `.github/dependabot.yml` no contiene entradas para ecosistema `cargo`
- CI verde en main post-commit

## Fuente de verdad

- `.github/dependabot.yml`
- `gh pr list --repo pablontiv/backscroll --state open --json number,title`
