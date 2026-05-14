---
estado: Completed
tipo: task
---
# T005: Update license from MIT to PolyForm Noncommercial 1.0.0

**Contribuye a**: standardize the ecosystem license — all 4 repos use PolyForm Noncommercial 1.0.0 (backscroll currently has MIT).

## Alcance

**In**:
- Replace `/LICENSE` content with PolyForm Noncommercial 1.0.0 full text, copyright "2026 Pablo Ontiveros"
- Update `README.md` license badge from MIT to PolyForm NC
- Update `README.md` License section text

**Out**:
- No changes to CI, code, or docs beyond the license badge/section

## Criterios de Aceptación

- `/home/shared/backscroll/LICENSE` contains "PolyForm Noncommercial License 1.0.0"
- README license badge links to PolyForm NC
- `git -C /home/shared/backscroll log --oneline -1` shows a conventional commit

## Fuente de verdad

- /home/shared/backscroll/LICENSE
- /home/shared/backscroll/README.md
- /home/shared/rootline/LICENSE (reference for PolyForm NC text)
