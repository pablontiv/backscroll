# S035: Output format enriquecido

**Feature**: [F01 Search Enriquecido](../README.md)
**Capacidad**: Output de search muestra formato enriquecido: [SESSION] fecha, slug, snippet con highlight, score.
**Cubre**: P1 del Epic (output enriquecido completo)

[[blocks:S034-fts5-snippet]]

## Antes / Despues

**Antes**: Output es bare `println!` con "Archivo:" y "Contenido:" en crudo. Score calculado pero nunca mostrado.

**Despues**: Output formateado: header `[SESSION] fecha · slug`, snippet con highlight markers convertidos a terminal bold/color, score visible. Test de snapshot con insta.

## Criterios de Aceptacion (semanticos)

- [ ] Output muestra fecha, slug, snippet, y score
- [ ] Snapshot test valida el formato exacto

## Invariantes

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Tasks

| Task | Descripcion |
|------|-------------|
| [T035](T035-format-output.md) | Formatear output con session header + snippet + score |
| [T036](T036-snapshot-test.md) | Test de snapshot del nuevo formato con insta |

## Fuente de verdad

- `src/main.rs` — output formatting (sera movido a output.rs en S036)
