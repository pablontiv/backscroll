---
estado: Completed
tipo: code
ejecutable_en: 1 sesion
---
# T012: Rediseñar SearchEngine trait

**Story**: [S024 Abstracciones de dominio](README.md)
**Contribuye a**: SearchEngine trait refleja el flujo real del sistema

[[blocks:T011-parsed-structs]]

## Preserva

- INV1: `cargo test --all-features` pasa
  - Verificar: `cargo test --all-features`
- INV2: `just check` pasa
  - Verificar: `just check`

## Contexto

El trait actual tiene `index_message(&self, path, role, content, project)` y `search(&self, query, project)`. No refleja el flujo real: sync produce archivos parseados, el engine los almacena. El trait debe recibir `Vec<ParsedFile>` y retornar `BackscrollError`.

## Especificacion Tecnica

```rust
pub trait SearchEngine {
    fn sync_files(&self, files: Vec<ParsedFile>) -> Result<(), BackscrollError>;
    fn search(&self, query: &str, project: &Option<String>) -> Result<Vec<SearchResult>, BackscrollError>;
    fn get_file_hashes(&self) -> Result<HashMap<String, String>, BackscrollError>;
}
```

## Alcance

**In**:
1. Rediseñar firmas del trait en `src/core/mod.rs`
2. Actualizar SearchResult: `{source_path, text, match_snippet: Option<String>, score: f64}`
3. Remover `#[allow(dead_code)]` del trait (se implementara en S025)

**Out**: No implementar para Database (S025). No refactorizar main.rs (S026).

## Estado inicial esperado

- ParsedFile y ParsedMessage definidos (T011)
- Trait actual con firmas legacy

## Criterios de Aceptacion

- Trait tiene `sync_files`, `search`, `get_file_hashes`
- SearchResult tiene `match_snippet: Option<String>`
- `cargo check` compila (trait sin implementors temporalmente con `#[allow(dead_code)]` si necesario)
- `just check` pasa

## Fuente de verdad

- `src/core/mod.rs`
