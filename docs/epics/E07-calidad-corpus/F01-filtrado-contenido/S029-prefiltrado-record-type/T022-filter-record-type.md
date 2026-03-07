---
estado: Pending
tipo: code
ejecutable_en: 1 sesion
---
# T022: Filtrar records por type en sync

**Story**: [S029 Pre-filtrado por record type](README.md)
**Contribuye a**: Solo records type=user y type=assistant se indexan

## Preserva

- INV1: Parse rate >= 95% en JSONL reales
  - Verificar: contar records parseados vs totales en test
- INV2: Sync incremental funciona
  - Verificar: `cargo test test_incremental_sync`

## Contexto

Los records JSONL de Claude Code tienen un campo `type` que indica su tipo: "user", "assistant", "progress", "system", etc. Solo "user" y "assistant" contienen contenido util para busqueda. Records con `isMeta: true` tambien deben descartarse. En corpus reales, ~51.6% son progress records — filtrarlos reduce significativamente el volumen.

## Alcance

**In**:
1. En parse_sessions, filtrar: solo procesar records donde `record.record_type` es "user" o "assistant"
2. Skip records con `record.is_meta == Some(true)`
3. Log con `tracing::debug!` los records skipped (conteo por tipo)

**Out**: No filtrar por contenido (eso es S030). Solo por tipo de record.

## Estado inicial esperado

- SessionRecord tiene campo `record_type` y `is_meta` (T001)
- parse_sessions procesa todos los records sin discriminar

## Criterios de Aceptacion

- `cargo test test_filter_by_type` — solo user/assistant en resultado
- Records progress/system se skipean
- `just check` pasa

## Fuente de verdad

- `src/core/sync.rs`
