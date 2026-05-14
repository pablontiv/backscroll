---
estado: Pending
tipo: task
---
# T006: Fix codeql.yml language from rust to go

**Contribuye a**: ensure CodeQL scans the actual language of the repo — backscroll was ported to Go (O06 completed) but codeql.yml still says `language: rust`.

## Alcance

**In**:
- Update `.github/workflows/codeql.yml`: change `language: rust` to `language: go`

**Out**:
- No other changes to CI workflows

## Estado inicial esperado

- `.github/workflows/codeql.yml` contains `language: rust`
- The repo main branch contains only Go code (Rust is frozen in the `v0` branch)

## Criterios de Aceptación

- `grep "language:" /home/shared/backscroll/.github/workflows/codeql.yml` returns `language: go`
- `git -C /home/shared/backscroll log --oneline -1` shows a conventional commit

## Fuente de verdad

- /home/shared/backscroll/.github/workflows/codeql.yml
