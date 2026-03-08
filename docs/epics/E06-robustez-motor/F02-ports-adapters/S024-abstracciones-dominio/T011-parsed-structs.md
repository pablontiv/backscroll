---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T011: Definir ParsedFile + ParsedMessage structs

**Story**: [S024 Abstracciones de dominio](README.md)
**Contribuye a**: ParsedFile y ParsedMessage structs definidas y usadas

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

Se necesitan structs intermedias que desacoplen el parsing (sync.rs) del storage (sqlite.rs). `ParsedFile` agrupa los mensajes parseados de un archivo JSONL con su metadata. `ParsedMessage` es un mensaje individual ya extraido del wrapper.

## Especificacion Tecnica

```rust
// src/core/mod.rs
pub struct ParsedMessage {
    pub role: String,
    pub text: String,
    pub ordinal: usize,
    pub uuid: Option<String>,
    pub timestamp: Option<String>,
}

pub struct ParsedFile {
    pub source_path: String,
    pub hash: String,
    pub project: Option<String>,
    pub messages: Vec<ParsedMessage>,
}
```

## Alcance

**In**:
1. Definir `ParsedFile` y `ParsedMessage` en `src/core/mod.rs`
2. Documentar campos con doc comments

**Out**: No modificar sync.rs ni sqlite.rs para usarlos (eso es S026).

## Estado inicial esperado

- `src/core/mod.rs` tiene SearchEngine trait y SearchResult

## Criterios de Aceptacion

- `grep "pub struct ParsedFile" src/core/mod.rs` encuentra el struct
- `grep "pub struct ParsedMessage" src/core/mod.rs` encuentra el struct
- `cargo check` compila
- `just check` pasa

## Fuente de verdad

- `src/core/mod.rs`
