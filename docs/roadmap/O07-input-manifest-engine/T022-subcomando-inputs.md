---
id: T022
tipo: task
estado: Pending
titulo: Subcomando inputs (list, aliases, identify, test)
outcome: O07
dependencias: [T015, T016, T019]
---

# T022 — Subcomando `inputs` (list, aliases, identify, test)

Implementar el subcomando `backscroll inputs` con los sub-subcomandos que existían
en v0 para inspeccionar y diagnosticar el motor de inputs.

## Alcance

En `cmd/backscroll/inputs.go`:

```
backscroll inputs list                    # lista todos los inputs activos con sus paths discovery
backscroll inputs aliases                 # muestra los aliases de cada input (nombre → path)
backscroll inputs identify <path>         # indica qué input manifest matchea un archivo dado
backscroll inputs test <path>             # aplica el pipeline completo a un archivo y muestra el resultado
```

- `inputs list`: nombre, enabled, format, discover.include patterns, count de archivos descubiertos
- `inputs aliases`: nombre → paths descubiertos (uno por línea)
- `inputs identify <path>`: qué manifest matchea el path dado (o "no match")
- `inputs test <path>`: corre decode → record → map → content → text sobre el archivo y
  muestra los registros que se indexarían (dry-run, no escribe en DB)
- Flags: `--json` para output JSON en todos los subcomandos

## Criterios de aceptación

- `backscroll inputs list` muestra ≥1 input cuando `claude.inputs.toml` está activo
- `backscroll inputs test ~/.claude/projects/.../session.jsonl` muestra el contenido parseado
- `backscroll inputs identify` retorna el nombre del manifest que matchea
- `--json` produce JSON válido
- `go test ./cmd/backscroll/...` pasa (test de integración básico)
