---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T041: Implementar subcomando read con noise filtering

**Story**: [S037 Modo --read](README.md)
**Contribuye a**: Noise filtering se aplica al leer

[[blocks:T040-reader-module]]

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` no se ve afectado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll read test.jsonl` < 1s

## Contexto

Agregar subcomando `read <path>` al CLI que lee una sesion JSONL individual y muestra los mensajes filtrados formateados.

## Alcance

**In**:
1. Agregar variante `Read { path: PathBuf }` al enum Commands en main.rs
2. Implementar handler: `read_session(path)` → formatear y mostrar cada mensaje
3. Formato: `[role] content` por mensaje, separados por linea vacia
4. Usar output.rs si aplica, o formato simple dedicado

**Out**: No agregar flags de formato a read (futuro).

## Estado inicial esperado

- core/reader.rs implementado (T040)
- CLI tiene 3 comandos (sync, search, status)

## Criterios de Aceptacion

- `backscroll read --help` muestra el subcomando
- `backscroll read session.jsonl` muestra contenido filtrado
- Mensajes de ruido no aparecen en output
- `just check` pasa

## Fuente de verdad

- `src/main.rs`
- `src/core/reader.rs`
