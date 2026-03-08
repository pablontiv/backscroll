---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T040: Crear core/reader.rs para lectura filtrada

**Story**: [S037 Modo --read](README.md)
**Contribuye a**: backscroll read session.jsonl muestra contenido filtrado

## Preserva

- INV1: Busqueda sin flags produce output legible
  - Verificar: `backscroll search "test"` no se ve afectado
- INV2: Performance < 1s en corpus de test
  - Verificar: `time backscroll read test.jsonl` < 1s

## Contexto

Modulo dedicado para leer una sesion JSONL individual y mostrar su contenido filtrado. Reutiliza el parser (SessionRecord) y los filtros de ruido de E07, pero no pasa por SQLite — lee directamente del archivo.

## Alcance

**In**:
1. Crear `src/core/reader.rs`
2. Funcion `read_session(path: &Path) -> Vec<ParsedMessage>`
3. Parsear JSONL → SessionRecord → filtrar por type → filtrar ruido → retornar mensajes limpios
4. Registrar modulo en core/mod.rs

**Out**: No implementar CLI subcommand (T041). No formatear output.

## Estado inicial esperado

- SessionRecord y filtros de ruido implementados (E06/E07)
- No existe modulo reader

## Criterios de Aceptacion

- `test -f src/core/reader.rs` — modulo existe
- `cargo test test_read_session` — lee fixture y retorna mensajes filtrados
- `just check` pasa

## Fuente de verdad

- `src/core/reader.rs` (nuevo)
- `src/core/mod.rs`
