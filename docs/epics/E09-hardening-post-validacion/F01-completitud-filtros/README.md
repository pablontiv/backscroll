# F01: Completitud de Filtros de Ruido

**Epic**: [E09 Hardening Post-Validacion](../README.md)
**Objetivo**: Completar los patrones de ruido faltantes del research y optimizar la compilacion de regex.
**Satisface**: P1 (todos los patrones filtrados), P3 (regex compilados una vez)
**Milestone**: `cargo test test_noise` pasa con cobertura de todos los patrones del research.

## Invariantes

- INV1: Parse rate >= 95% en JSONL reales (heredado de E09/E07)
- INV2: `just check` pasa (heredado de E09)
- INV3: Tests existentes no regresan (heredado de E09)

## Stories

| Story | Descripcion |
|-------|-------------|
| [S040](S040-patrones-faltantes/) | Patrones de ruido faltantes |
| [S041](S041-optimizacion-regex/) | Optimizacion de compilacion regex |
