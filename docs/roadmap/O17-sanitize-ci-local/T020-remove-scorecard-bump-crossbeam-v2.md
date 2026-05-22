---
estado: Specified
tipo: task
---
# T020: Remover workflow Scorecard local y bumpear a `crossbeam@v2`

**Outcome**: [O17 Sanitize CI local](README.md)
**Contribuye a**: eliminar 67% de las fallas tipo startup_failure observadas en backscroll y heredar los cambios saneados de crossbeam (coverage default=0, scorecard removido del set reusable).

## Preserva

- INV1: CodeQL, gitleaks y go-release siguen funcionando vía crossbeam reusable.
  - Verificar: `gh run list --repo pablontiv/backscroll --workflow codeql.yml --limit 3` y `--workflow go-release.yml` retornan success post-merge.
- INV2: pre-push hook local (`just coverage-check`) y `.coverage-floors.toml` no se tocan.

## Contexto

Backscroll usa `pablontiv/crossbeam@v1` para CI/release y tiene un workflow `scorecard.yml` que llama al reusable. Tras la publicación de `crossbeam@v2` (outcome O01), backscroll debe:
1. Eliminar la llamada a scorecard (no existirá en v2).
2. Bumpear todas las referencias `@v1` → `@v2`.

Dependencia cross-repo: requiere `crossbeam@v2` tageado y publicado. No se usa `blocked_by` formal porque el target está fuera del roadmap de backscroll; se valida en AC ("v2 tag exists").

## Alcance

**In**:
1. Eliminar `.github/workflows/scorecard.yml` local.
2. Buscar todas las referencias `pablontiv/crossbeam/.../@v1` en `.github/workflows/*.yml` y bumpear a `@v2`.
3. Actualizar `CLAUDE.md` sección "CI/CD" para remover scorecard.yml de la tabla.

**Out**:
- No tocar lógica de tests, `.coverage-floors.toml`, pre-push hook, ni `Justfile`.
- No modificar el flow de release.

## Estado inicial esperado

- `crossbeam@v2` publicado (verificar: `gh release view v2 --repo pablontiv/crossbeam`).
- `.github/workflows/scorecard.yml` existe.
- Workflows referencian `@v1`.

## Criterios de Aceptación

- `ls .github/workflows/scorecard.yml` retorna "No such file or directory".
- `grep -rE 'pablontiv/crossbeam/.*@v1' .github/workflows/` retorna 0 matches.
- `grep -rE 'pablontiv/crossbeam/.*@v2' .github/workflows/` retorna al menos 1 match.
- `CLAUDE.md` sección "CI/CD" ya no menciona scorecard.yml.
- Próximo push a main: `gh run list --repo pablontiv/backscroll --branch main --limit 3` retorna `conclusion=success`.

## Fuente de verdad

- `/home/shared/backscroll/.github/workflows/`
- `/home/shared/backscroll/CLAUDE.md`
