---
estado: Pending
tipo: test
ejecutable_en: 1 sesion
---
# T036: Test de snapshot del nuevo formato con insta

**Story**: [S035 Output format enriquecido](README.md)
**Contribuye a**: Snapshot test valida el formato exacto

[[blocks:T035-format-output]]

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` produce output formateado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll search "test"` < 1s

## Contexto

Usar insta (ya en dependencias) para snapshot testing del formato de output. Garantiza que cambios futuros al formato se detectan y revisan explicitamente.

## Alcance

**In**:
1. Test que formatea un SearchResult conocido y compara con snapshot
2. `cargo insta review` para aprobar snapshot inicial
3. Test para modo TTY (con bold) y modo pipe (sin bold)

**Out**: No agregar tests para --json/--robot (S036).

## Estado inicial esperado

- Output formateado implementado (T035)
- insta crate disponible en dev-dependencies

## Criterios de Aceptacion

- `cargo test test_output_format_snapshot` pasa
- Snapshot file existe en snapshots/
- `cargo insta review` no tiene pendientes
- `just check` pasa

## Fuente de verdad

- `tests/` o tests unitarios en main.rs
