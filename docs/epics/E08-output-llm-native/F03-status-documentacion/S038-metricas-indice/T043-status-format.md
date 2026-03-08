---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T043: Formatear status con metricas, test snapshot

**Story**: [S038 Metricas reales del indice](README.md)
**Contribuye a**: backscroll status muestra conteo y tamano de DB

[[blocks:T042-status-queries]]

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` no se ve afectado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll status` < 1s

## Contexto

Formatear las metricas de Stats en output legible para el comando status. Snapshot test para validar formato.

## Especificacion Tecnica

```
Backscroll Index Status
  Files indexed:  142
  Messages:       12,847
  Projects:       5
  Database size:  8.2 MB
  Last sync:      2026-03-07 10:23:45
```

## Alcance

**In**:
1. Reemplazar stub de status en main.rs con llamada a get_stats()
2. Formatear output con alineacion y separadores legibles
3. Human-readable sizes (KB, MB, GB)
4. Snapshot test con insta

**Out**: No agregar --json a status (futuro).

## Estado inicial esperado

- get_stats() implementado (T042)
- Status es un stub

## Criterios de Aceptacion

- `backscroll status` muestra metricas formateadas
- `cargo test test_status_format_snapshot` pasa
- `just check` pasa

## Fuente de verdad

- `src/main.rs`
