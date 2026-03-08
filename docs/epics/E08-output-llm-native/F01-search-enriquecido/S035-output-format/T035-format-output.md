---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T035: Formatear output con session header + snippet + score

**Story**: [S035 Output format enriquecido](README.md)
**Contribuye a**: Output muestra fecha, slug, snippet, y score

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Contexto

Reemplazar el output bare actual (`println!("Archivo: {}", res.path)`) con formato enriquecido que muestre contexto util: header con fecha y slug de sesion, snippet con highlight, y score de relevancia.

## Especificacion Tecnica

```
[SESSION] 2026-03-01 · project-slug
  ...context before >>>matched term<<< context after...
  Score: 0.85

[SESSION] 2026-03-02 · another-slug
  ...different >>>match<<< here...
  Score: 0.72
```

Markers `>>>` y `<<<` se convierten a terminal bold (si TTY) o se mantienen como texto (si pipe).

## Alcance

**In**:
1. Reemplazar println! de search results en main.rs
2. Formatear header: `[SESSION] timestamp · source_path_slug`
3. Formatear snippet con markers → bold (detectar TTY)
4. Mostrar score

**Out**: No extraer a modulo output.rs (S036). Implementar inline en main.rs primero.

## Estado inicial esperado

- SearchResult tiene match_snippet: Some (S034)
- Output es bare println!

## Criterios de Aceptacion

- `backscroll search "test"` muestra header con fecha
- `backscroll search "test"` muestra snippet
- Score visible en output
- `just check` pasa

## Fuente de verdad

- `src/main.rs`
