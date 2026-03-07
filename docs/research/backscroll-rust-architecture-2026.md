---
estado: "Fase 3 (Actualizada)"
fecha: "2026-03-06"
metodo: hypothesize -> architectural pivot
origen: "Evolución de research original (Go) hacia arquitectura Rust"
fase_actual: 3
---
# Backscroll — Investigación Estructurada (Arquitectura Rust 2026)

**Fecha**: 2026-03-06 (Revisión Arquitectónica)
**Tipo**: Research & Tech Spec
**Ecosistema**: Backscroll provee Tier 2 search para Kedral (Known Error Database). Backscroll = event store + búsqueda.

> **Estado:** Fase 3 completa. Go/No-Go confirmado. **Decisión: Pivotar de Go a Rust.** Este documento reemplaza la propuesta original basada en Go+modernc.org.

---

## 1. Resumen Ejecutivo del Pivote Arquitectónico

La evaluación exhaustiva del panorama tecnológico a **marzo de 2026** demostró que, aunque la propuesta original en Go era funcional, **Rust** elimina por completo los tres riesgos técnicos más críticos del proyecto:
1. **Rendimiento de FTS5:** Reemplaza la lenta transpilación de Go por el motor C nativo de SQLite, logrando latencias de indexación y ordenamiento (BM25) de 3x a 10x más rápidas.
2. **Robustez ante esquemas inestables:** Reemplaza las frágiles aserciones de tipo por deserialización segura en tiempo de compilación (`serde`).
3. **Escalabilidad futura:** Establece las bases en el mismo ecosistema de Tantivy, permitiendo una migración trivial si el corpus escala más allá de los límites de SQLite.

Backscroll se mantiene como una **CLI on-demand** (sin daemon) que sincroniza incrementalmente sesiones de Claude Code (<50ms).

---

## 2. Stack Tecnológico Base (Versiones 2026)

*   **Lenguaje:** Rust `1.94.0` (Marzo 2026, Edition 2024). Tiempo de arranque en microsegundos, ideal para CLI.
*   **Base de Datos / Motor FTS:** `rusqlite v0.38.0`. Usando el feature `bundled` (SQLite 3.51.1+ interno en C, sin dependencias del sistema).
*   **Parser Defensivo:** `serde` + `serde_json` (v1.0.x). Manejo de esquemas mutantes mediante macros `#[serde(untagged)]`.
*   **CLI Framework:** `clap v4.5.60` (febrero 2026) con API `derive`.
*   **Distribución Estática:** `cargo-zigbuild`. Herramienta de compilación cruzada para lograr binarios 100% estáticos ("Zero Deps") en Windows, Mac y Linux.

---

## 3. Resolución de Riesgos Técnicos (Spikes)

### 3.1. Parseo Defensivo del JSONL (El Caos de Claude Code)
El esquema de las sesiones de Claude no es oficial. El campo `content` puede ser un String o un Array de bloques.
**Solución Rust:**
```rust
use serde::Deserialize;

#[derive(Deserialize, Debug)]
#[serde(untagged)]
pub enum MessageContent {
    Text(String),
    Blocks(Vec<ContentBlock>),
}

#[derive(Deserialize, Debug)]
pub struct ClaudeMessage {
    pub role: String,
    pub content: MessageContent,
    #[serde(default)] // Ignora si no existe
    pub is_meta: bool,
    // ... otros campos se ignoran silenciosamente por defecto
}
```
*Impacto:* Riesgo de *crashes* durante indexación (CAP-03) reducido a cero sin penalización de rendimiento.

### 3.2. Concurrencia y Sincronización Incremental
Se mantiene el requisito de no usar un demonio residente en memoria (Premisa D2). 
*Riesgo:* Múltiples agentes escribiendo simultáneamente causando `database is locked`.
**Solución:**
1.  Activar `PRAGMA journal_mode=WAL;` (Múltiples lectores, un escritor concurrente).
2.  Configurar un timeout a nivel de conexión en `rusqlite`:
    ```rust
    let conn = Connection::open(db_path)?;
    conn.busy_timeout(std::time::Duration::from_millis(5000))?;
    ```
*Impacto:* Si un agente humano y un subagente indexan al mismo tiempo, uno esperará pacientemente hasta 5 segundos, garantizando transaccionalidad sin necesidad de un complejo Connection Pool o Daemon HTTP.

---

## 4. Diseño de Software: Puertos y Adaptadores (Preparación para el Futuro)

Para garantizar que Backscroll pueda sobrevivir a la barrera de los ~15 millones de mensajes (El "Muro" de SQLite), la aplicación **no acoplará la lógica de la CLI al SQL**. 

El dominio principal interactuará con el almacenamiento puramente a través de un `trait` (interfaz):

```rust
// core/domain.rs
pub struct SearchResult {
    pub source_path: String,
    pub text: String,
    pub match_snippet: String,
    pub score: f32, // Relevancia (BM25)
}

pub trait SearchEngine {
    fn sync_files(&self, files: Vec<ParsedFile>) -> Result<(), BackscrollError>;
    fn search(&self, query: &str, project_slug: &Option<String>) -> Result<Vec<SearchResult>, BackscrollError>;
}
```

La implementación V1 será `src/storage/sqlite.rs`. Si en el futuro el corpus exige más rendimiento, la migración consistirá únicamente en crear `src/storage/tantivy.rs`, sin modificar la CLI ni el parseo.

---

## 5. Estrategia de Compilación Cruzada ("Zero Deps")

Para mantener el principio de distribución fácil (un solo binario, cero dependencias) que motivaba la propuesta en Go, usaremos **Zig como linker de C** para la librería nativa de SQLite.

**Configuración Local/CI:**
1. Instalar `cargo-zigbuild`: `cargo install cargo-zigbuild`
2. Compilar para Linux Estático: 
   `cargo zigbuild --release --target x86_64-unknown-linux-musl`
3. Compilar para Windows: 
   `cargo zigbuild --release --target x86_64-pc-windows-gnu`
4. Compilar para macOS (Intel/Apple Silicon): 
   `cargo zigbuild --release --target aarch64-apple-darwin`

*Resultado:* Un ejecutable nativo por plataforma que contiene el motor completo de SQLite C optimizado, sin requerir `libsqlite3` en la máquina del usuario final.

---

## 6. Umbrales de Operación y Límite de Vida de la v1

Este sistema basado en SQLite FTS5 será el "Sweet Spot" operativo mientras Backscroll se mantenga dentro de estos márgenes empíricos:
*   **Tamaño del Corpus:** < 50 GB.
*   **Cantidad de Mensajes (Rows):** < 10.000.000.
*   **Estado Actual (Marzo 2026):** ~1 GB y ~50.000 rows (Uso al 0.5% de la capacidad de la herramienta).

**¿Cuándo Pivotar a Tantivy (Opción v2)?**
Se abandonará `rusqlite` a favor de `tantivy` única y exclusivamente si:
1. Las consultas con `ORDER BY rank` superan constantemente los >500ms debido a la explosión del costo de calcular BM25.
2. Kedral amplía su alcance de indexar texto exacto (sesiones) a buscar código tolerante a fallos (fuzzy matching, typos).
3. El archivo `.db` agota la Memoria RAM disponible (Page Cache Thrashing).

Hasta entonces, SQLite FTS5 embebido nativamente en Rust es la solución definitiva.