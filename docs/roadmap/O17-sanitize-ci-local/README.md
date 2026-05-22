---
tipo: outcome
---
# Sanitize CI local — remover Scorecard + estabilizar test flake

Eliminar las dos fuentes específicas de fricción CI en backscroll (Scorecard 67% startup_failure + `TestResumeOutputFormatRobot` flake) y bumpear al reusable `crossbeam@v2` una vez publicado.

## Contexto

Diagnóstico cuantitativo (ver plan global en `/home/pones/.claude/plans/tenemos-multiples-incluso-cientos-sprightly-valley.md`) sobre backscroll mostró:
- Tasa de falla CI ~42% en los últimos 50 runs.
- Scorecard con 67% `startup_failure` (4 fallas en 6 runs) — sin valor entregado.
- Test `TestResumeOutputFormatRobot` falla intermitente con "No relevant sessions found for: test" — setup/teardown fragility, no bug funcional.

Coordinado con `crossbeam/O01-sanitize-ci-reusables`: una vez `crossbeam@v2` esté publicado (Scorecard removido del set + coverage default=0), backscroll bumpea y limpia su workflow local.
