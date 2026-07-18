# Pattern Detection Technologies: Research Report

Date: 2026-07-16  
Scope: Identifying pattern-detection technologies for code and documentation beyond full-text search  
Status: **RESEARCH COMPLETE**

---

## Executive Summary

Pattern detection in source code and documentation spans six distinct technology families, each with a different definition of "pattern" and retrieval semantics:

1. **Structural/AST Pattern Matching** — code patterns as abstract syntax trees (Semgrep, ast-grep, CodeQL)
2. **Code Intelligence & Symbol Graphs** — semantic understanding via indexed references and type information (Sourcegraph, tree-sitter-graph)
3. **Code Clone & Duplication Detection** — finding repeated code blocks (PMD CPD, jscpd)
4. **Semantic/Embedding-Based Detection** — vector-space similarity over embeddings (sqlite-vec, LanceDB + embedding models)
5. **Documentation Pattern Tools** — prose style rules and structural patterns (Vale)
6. **Event-Store Pattern Mining** — discovering patterns in session/event logs (theoretical extension to backscroll model)

**Key Finding**: All six families are production-ready and locally deployable; none require cloud SaaS. Relevance to backscroll depends on the pattern scope: sessions as structured events (families 1–2 apply directly); sessions as prose content (family 5); sessions as semantic queries (family 4).

---

## 1. Structural/AST Pattern Matching

### Overview

AST-based pattern matching treats source code as structured trees, not text. Patterns are written in code syntax with metavariables and wildcard operators. This enables finding patterns that are semantically equivalent even when formatted or named differently.

**Key distinction**: structural patterns find code *by structure* (e.g., "any function that calls X without checking the return value"), not by regex or keywords.

### Technology Comparison

| Technology | Language Support | Pattern Language | Local/SaaS | License | CGO/Dependencies | Go Bindings |
|---|---|---|---|---|---|---|
| **Semgrep** | 30+ (Python, JS, Go, Java, C/C++, etc.) | Semgrep rule YAML (code-like syntax) | Local + SaaS | SSPL (Pro) / Proprietary | Go CLI only (no SDK) | No native Go SDK |
| **ast-grep** | 25+ (via tree-sitter) | Pattern code (natural code syntax) | Local only | MIT | No (Rust binary) | Go bindings available |
| **CodeQL** | 9 (C/C++, C#, Go, Java, JS, Python, Ruby, TS, Swift) | QL (SQL-like relational query) | Local + SaaS | Proprietary | C/C++ backend | Python/JavaScript only |

### Semgrep
[https://semgrep.dev/](https://semgrep.dev/)

**What it is**: Lightweight static analysis tool that parses code into ASTs and matches patterns written as rule YAML.

**Pattern Model**: 
- **Metavariables** (`$X`, `$LOGGER`) match unknown values; metavariables with the same name must match identical content
- **Ellipsis** (`...`) matches zero or more items (function arguments, statements, fields)
- **Deep expressions** (`<... pattern ...>`) match nested patterns recursively
- **Pattern operators**: `pattern-inside`, `pattern-either`, `pattern-not`, `metavariable-pattern`

**Local Deployment**: Semgrep CLI is a single Go binary (~50 MB); no runtime required. Rules are YAML files in `~/.semgrep/` or `.semgrep.yml` at repo root.

**Integration Path**: Shell invocation; no in-process SDK for Go. Would require subprocess spawning or scraping CLI output.

---

### ast-grep
[https://ast-grep.github.io/](https://ast-grep.github.io/)

**What it is**: Fast structural search and rewriting tool written in Rust. Patterns are written as ordinary code with metavariable placeholders.

**Pattern Model**:
- **Patterns as code**: `console.log($$$ARGS)` matches any console.log call with any arguments
- **Metavariables** (`$X`, `$$X`, `$$$X`) for single nodes, named sequences, and unnamed sequences
- **Node matching**: `$$$ X` matches zero or more nodes; `$X` matches one node
- **Context matching**: patterns can target specific language constructs

**Local Deployment**: Single Rust binary (~10 MB), no dependencies, runs on Linux/macOS/Windows/WASM/Raspberry Pi. Configuration via `sgconfig.yaml`.

**Integration Path**: CLI tool with JSON output; no native Go SDK. Go language bindings exist but require cgo.

---

### CodeQL
[https://codeql.github.com/](https://codeql.github.com/)

**What it is**: Proprietary static analysis framework (acquired by GitHub from Semmle). Transforms source code into a relational database and queries it using QL, a SQL-like query language.

**Pattern Model**:
- **QL Language**: Declarative, object-oriented query language similar to SQL
- **Data flow analysis**: Can trace values across function boundaries and control flow
- **Semantic patterns**: Queries express conditions over the full code graph (classes, methods, variables, control structures, data paths)
- **Example**: "Find any loop inside function foo() where the loop body calls bar() without checking the result"

**Local Deployment**: GitHub-hosted; available locally via `codeql` CLI for scanning repos. Analysis requires a GitHub account for some features; free for public repos.

**Integration Path**: CLI tool with JSON/SARIF output. No Go SDK; Python/JavaScript SDKs available.

---

## 2. Code Intelligence & Symbol Graphs

### Overview

Symbol graphs capture semantic relationships between code elements: function calls, type definitions, imports, variable assignments. They enable queries like "all callers of this function" or "all types implementing this interface" without parsing.

**Key distinction**: indexed references enable *recall* (finding all uses of a symbol), not just pattern matching.

### Technology Comparison

| Technology | Foundation | Local/SaaS | License | Focus | Go Support |
|---|---|---|---|---|---|
| **Sourcegraph** | SCIP index | SaaS (can self-host) | Proprietary | Code search + AI context | Full |
| **tree-sitter-graph** | tree-sitter parser | Local | MIT | Graph construction DSL | Via Rust bindings |

### Sourcegraph
[https://sourcegraph.com/](https://sourcegraph.com/)

**What it is**: Code intelligence platform built on SCIP (Specification for Code Intelligence Protocol) indexes. Provides cross-reference lookup, definition navigation, and semantic code search.

**Pattern Model**:
- **Symbol-based search**: Find definitions, references, implementations by name or type
- **Cross-file queries**: "All callers of Function X" across the entire codebase
- **Hover tooltips**: Type information, documentation, usage context
- **Cody AI context**: Feeds semantic code context to AI assistants via MCP server

**Deployment Model**: SaaS (sourcegraph.com) or self-hosted via Docker. Scans public/private repos continuously.

**Integration Path**: REST API for queries; MCP server for AI context injection. No direct Go library (would integrate via HTTP).

**Relevance to backscroll**: Sourcegraph's "code as data" philosophy mirrors backscroll's "sessions as events." Provides a model for injecting semantic context into AI workflows.

---

### tree-sitter-graph
[https://github.com/tree-sitter/tree-sitter-graph](https://github.com/tree-sitter/tree-sitter-graph)

**What it is**: Domain-specific language (DSL) for constructing arbitrary graph structures from tree-sitter parse trees. Used to build symbol graphs, call graphs, and type graphs.

**Pattern Model**:
- **Declarative graph construction**: Define nodes and edges by pattern matching on AST nodes
- **Pattern-based rules**: `(node type: "function_definition") @func` captures function definition nodes
- **Graph relationships**: Rules define edges (e.g., "function A calls function B")

**Local Deployment**: Rust library; can be compiled into custom tools. No prebuilt Go bindings.

**Integration Path**: Wrap tree-sitter-graph in a subprocess (Rust binary) or write custom Go parser using `github.com/tree-sitter/go-tree-sitter`.

---

## 3. Code Clone & Duplication Detection

### Overview

Clone detection finds repeated code blocks, enabling refactoring opportunities and complexity analysis. Unlike AST matching, clone detectors *discover* patterns without pre-specifying them.

**Key distinction**: unsupervised (finds all duplicates) vs. supervised (searches for known patterns).

### Technology Comparison

| Technology | Languages | Min Unit | Algorithm | Tokenizer | Local | AI-Ready | Single Binary |
|---|---|---|---|---|---|---|---|
| **PMD CPD** | Java, JSP, C, C++, Fortran, PHP | 100 tokens (configurable) | Token sequence matching | Lexical tokenizer | Yes | No | JAR (requires JVM) |
| **jscpd** | 223 formats | 5-10 lines (tunable) | Rabin-Karp rolling hash | Language-specific | Yes | Yes (MCP/reporters) | Yes (Rust, pre-built) |

### PMD CPD
[https://pmd.github.io/pmd/pmd_userdocs_cpd.html](https://pmd.github.io/pmd/pmd_userdocs_cpd.html)

**What it is**: Copy/Paste Detector from the PMD static analysis suite. Tokenizes code and finds repeated token sequences above a minimum length.

**Detection Model**:
- **Type 1 clones**: Exact duplicates
- **Type 2 clones**: Identical code with different identifiers/literals
- **Some Type 3 clones**: Minor structural differences (with parameter tuning)
- **Configurable minimum**: Default 100 tokens for Java, 120 for JavaScript; can be lowered

**Local Deployment**: JAR binary (`pmd-bin-<version>.zip`). Requires Java; integrates with Maven/Gradle via plugins.

**Limitations**:
- Requires JVM (violates pure-Go constraint for in-process activation)
- Fixed language list; less flexible than tree-sitter-based tools

---

### jscpd
[https://github.com/kucherenko/jscpd](https://github.com/kucherenko/jscpd)

**What it is**: Fast copy/paste detector with Rust-powered core (24–37x faster than legacy Node.js version). Supports 223 file formats via tree-sitter and language-specific tokenizers.

**Detection Model**:
- **Rabin-Karp algorithm**: Rolling hash for O(n) linear-time duplicate detection
- **Configurable detection**: `--min-tokens` (default 5), `--min-lines`, `--threshold %`
- **AI-optimized reporters**: Token-efficient JSON output designed for LLM pipelines

**Local Deployment**: Single pre-built binary (macOS, Linux, Windows); installable via Homebrew, Cargo, or direct download. Zero runtime dependencies.

**AI Integration**:
- **13+ reporters** (console, json, xml, csv, html, markdown, AI reporter)
- **MCP server**: Can be integrated into Claude Code and other AI agents
- **Skill available**: Community skill on `skills.rest` for pattern discovery

**Relevance to backscroll**: jscpd's AI reporter and token-efficient output make it a candidate for indexing duplication patterns in session histories. The MCP server enables direct integration into Claude workflows.

---

## 4. Semantic/Embedding-Based Pattern Detection

### Overview

Embedding-based detection maps code and documentation to dense vectors, enabling similarity search in semantic space rather than keyword/structure space. Useful for finding conceptually similar code across different implementations.

**Key distinction**: *semantic* (meaning-based) vs. *lexical* (text-based) or *structural* (AST-based).

### Technology Comparison

| Technology | Type | Embedding Model | Local | Language | Vector Scale |
|---|---|---|---|---|---|
| **sqlite-vec** | Vector index extension | User-supplied embeddings | Yes (SQLite) | C (all languages via bindings) | 10K–100K vectors |
| **LanceDB** | Vector database | User-supplied embeddings | Yes (Python/TypeScript/Rust) | Python/TypeScript/Rust | 1M–10M vectors |
| **Ollama** (sidecar) | Embedding provider | Sentence transformers (all-MiniLM, etc.) | Yes (HTTP API) | Go/Python/JS/etc. | Real-time on-demand |

### sqlite-vec
[https://github.com/asg017/sqlite-vec](https://github.com/asg017/sqlite-vec)

**What it is**: SQLite extension (written in pure C) that adds vector data types and KNN (k-nearest neighbor) search. Stores vectors alongside metadata in the same database.

**Pattern Model**:
- **Vector storage**: Native `vec0` virtual tables for float32, int8, and binary vectors
- **KNN queries**: `SELECT * FROM vec_table ORDER BY vec_distance(embedding, query_vector) LIMIT k`
- **Metadata filtering**: Combine vector similarity with SQL filters (e.g., "closest 10 vectors in project X")

**Local Deployment**: Pure C, no dependencies, runs anywhere SQLite runs (Linux, macOS, Windows, browser WASM, Raspberry Pi). Load as a SQLite extension: `.load ./vec0`.

**Limitations**: 
- Vectors must be supplied externally (no built-in embedding model)
- Suitable for 10K–100K vectors; less optimized for millions

**Integration Path for Backscroll**: Backscroll could extend its FTS5 database with `vec0` to store embedding vectors alongside search_items. Hybrid BM25 + vector search via Reciprocal Rank Fusion (RRF) is feasible; backscroll already implements RRF for tool_fts + messages_fts fusion (migration v7).

---

### LanceDB
[https://docs.lancedb.com/](https://docs.lancedb.com/)

**What it is**: Embedded vector database (open source, MIT license) with Python/TypeScript/Rust SDKs. Designed for offline-first AI applications.

**Pattern Model**:
- **Vector similarity search**: SQL + vector search combined (`SELECT * WHERE distance < threshold ORDER BY similarity`)
- **Multimodal**: Handles text, images, vectors, metadata in one table
- **FTS integration**: Can combine full-text search with vector search on same table
- **Auto-indexing**: IVFPQ (Inverted File with Product Quantization) for fast million-vector scale

**Local Deployment**: Pure Python/Rust library (pip install lancedb); stores data in local Parquet files. No server required.

**Embedding Model**: User supplies via Hugging Face, OpenAI, or local model (e.g., ollama). LanceDB provides helpers for common providers.

**Limitations**: Primary SDKs are Python/TypeScript; Go support is indirect (via subprocess or Python interop).

---

### Ollama (Embedding Provider Sidecar)
[https://ollama.ai/](https://ollama.ai/)

**What it is**: Local inference engine for embedding and language models. Runs as HTTP API sidecar; backscroll remains pure Go but spawns Ollama as a subprocess.

**Embedding Models**:
- `all-MiniLM-L6-v2`: 384-dimensional sentence embeddings, ~35 MB model
- `nomic-embed-text`: 768-dimensional, ~260 MB, specialized for long context
- Custom quantized models via `.gguf` format

**Local Deployment**: Single binary download (~200–400 MB); models download on first use (~30–40 MB for all-MiniLM). HTTP API on `localhost:11434`.

**Latency Profile** (from M1 embeddings spike):
- Cold start (first embedding): 50–100ms
- Warm requests (model in RAM): 10–30ms per embedding
- HTTP overhead: 5–10ms per request
- **Total for 20 queries**: ~300–600ms (acceptable for async indexing)

**Process Management**: Go `os/exec` sufficient; cleanup via signal handling.

**Relevance to backscroll**: Backscroll's M1 embeddings spike (2026-07-03) deferred activation due to 100% recall@5 on BM25-only baseline. However, for M2 (50-query diverse benchmark), real embeddings become a candidate if lexical search falls below 95% recall.

---

## 5. Documentation Pattern Tools

### Overview

Documentation-specific pattern detection focuses on prose style, structure, and quality. Rules are expressed as YAML patterns matching text, not code.

### Vale
[https://vale.sh/](https://vale.sh/)

**What it is**: Open-source prose linter that checks documentation against configurable style guide rules. Rules are YAML files; styles are folders of related rules.

**Pattern Model**:
- **Existence rules**: Flag presence/absence of specific words or patterns
- **Repetition rules**: Find repeated terms within a window
- **Spelling rules**: Check against dictionaries
- **Swap rules**: Enforce preferred terminology (`realize` → not `realise`)
- **Regex patterns**: Custom regex-based rules for structural checks

**Configuration**: Single `.vale.ini` at repo root specifies styles, minimum alert level, and which file types to lint.

**Included Styles**:
- Microsoft Manual of Style
- Google Developer Documentation Style Guide
- Alex (inclusive language)
- Custom house rules

**Local Deployment**: Single binary (macOS, Linux, Windows); installable via Homebrew or download. Runs as CLI; integrates into git hooks and CI.

**Integration Path**: CLI invocation; JSON/HTML output for parsing. No SDK.

**Relevance to backscroll**: Sessions contain prose (chat transcripts, reasoning blocks). Vale could lint for consistency across session content, style guide compliance, or terminology standardization. Less directly applicable than code-focused tools, but useful for documentation quality patterns.

---

## 6. Event-Store Pattern Mining (Backscroll-Specific)

### Overview

Backscroll treats sessions as an event store with structured logs. Pattern detection in this domain differs from code/docs: the "patterns" are recurring workflows, error chains, or decision paths across sessions.

### Applicable Patterns in Backscroll Sessions

**Temporal patterns**:
- "Tool use followed immediately by error" → debugging workflow
- "Search query → no results → reformulation" → search quality issue
- "Agent handoff → long pause → manual correction" → integration friction

**Semantic patterns**:
- "Session contains reasoning block with word X and code block with Y" → feature/debugging work
- "Multiple sessions reference the same source file and error" → persistent bug or refactoring
- "Session tags 'feature' and 'test'" → test-driven development workflow

**Quantitative patterns**:
- "Session with >50 tool uses" → complex task
- "Session involving 3+ agents" → orchestration complexity
- "High reasoning block density" → novel/exploratory work

### Detection Approaches

1. **Structured Query** (easiest): Filter sessions by structured metadata already in backscroll.
   - Example: `SELECT COUNT(*) FROM search_items WHERE content_type='reasoning' AND timestamp > ...`
   - Implemented: backscroll's tagging system (`internal/tagging`) already infers categories (debugging, refactoring, feature, testing, docs, config)

2. **Text Pattern** (moderate): Regex or keyword matching on session content.
   - Example: "Find all sessions mentioning 'database locked'" via full-text search
   - Implemented: backscroll's FTS5 search (`search --text 'database locked'`)

3. **Semantic Similarity** (advanced): Embedding-based discovery.
   - Example: "Find sessions semantically similar to this bug report"
   - Requires: sqlite-vec extension + embeddings (M2 consideration)

4. **Sequence Mining** (research-grade): Discover frequent event chains without pre-specification.
   - Example: Apriori algorithm on session event logs to find common tool-use sequences
   - Tools: Python libraries (mlxtend, PyFrequist) or research databases (GraphDB, Neo4j)
   - Feasibility: Low without moving to a graph database; not cost-effective for backscroll scale

---

## Comparison Table: Which Tool for Which Task?

| Task | Best Tools | Pattern Type | Effort | Local-First |
|---|---|---|---|---|
| Find code with same structure, different names | Semgrep, ast-grep, CodeQL | Structural | Low | Yes |
| Find all uses of a function across repos | Sourcegraph, tree-sitter-graph | Semantic/Reference | Medium | Yes (self-hosted) |
| Find duplicated code blocks | jscpd, PMD CPD | Lexical/Clone | Low | Yes |
| Find conceptually similar code | sqlite-vec + embeddings, LanceDB | Semantic/Embedding | High | Yes (with Ollama sidecar) |
| Enforce documentation style rules | Vale | Prose/Style | Low | Yes |
| Find debugging workflows in sessions | Backscroll search + tagging, structured queries | Metadata/Temporal | Low | Yes |
| Find semantically similar sessions | Backscroll FTS5 (current), or sqlite-vec + embeddings | Semantic/Lexical | Medium (with embeddings) | Yes |

---

## Relevance to Backscroll

### Immediate Applicability (v2.x)

**Already implemented via existing backscroll infrastructure**:
- **Auto-tagging** (internal/tagging): Heuristic pattern detection for debugging, refactoring, feature, testing, docs, config work
- **FTS5 search**: Lexical pattern detection via BM25 (tool content in separate trigram index for substring matching)
- **Session filtering**: Structured queries on timestamp, project, content_type

### M2 Candidates (50-query benchmark phase)

1. **Hybrid BM25 + embedding search** (medium effort):
   - Extend storage with sqlite-vec for embedding vectors
   - Use Ollama sidecar for real embedding generation
   - Implement RRF fusion (backscroll already does this for tool_fts + messages_fts)
   - Activation condition: M2 benchmark shows BM25 recall <95%

2. **Clone detection in session workflows** (low effort):
   - Integrate jscpd as subprocess to find repeated command sequences, code patterns within session logs
   - Output: "3 sessions executed identical refactoring pattern in different projects"

3. **Cross-session pattern mining** (research):
   - Structured queries to find temporal patterns (error sequences, agent hand-offs)
   - Example: "Find sessions with tool errors followed by manual fixes" via tagging + search

### Constraints & Notes

- **Pure-Go requirement**: Eliminates PMD CPD (Java) and direct LanceDB/Python integration. Workaround: subprocess spawning (jscpd, Ollama) or HTTP API (Sourcegraph, LanceDB server mode).
- **CGO constraint**: Tree-sitter graph and some Go bindings require cgo. Use subprocess wrappers or pre-built binaries instead.
- **No new external SaaS**: All recommended tools run locally or as open-source self-hosted services.

---

## Recommendation Summary

**For pattern detection in backscroll sessions**:

1. **Short term** (v2.x): Leverage existing FTS5 + tagging. Add jscpd for clone detection in workflows (low-effort CLI wrapper).
2. **Medium term** (M2): Evaluate sqlite-vec + Ollama sidecar for hybrid search if M2 benchmark shows BM25 gaps.
3. **Long term** (research): Explore event-stream analytics (Apache Kafka, temporal databases) for real-time workflow pattern discovery; out of scope for v2.x.

**For external code repositories** (if backscroll expands to index external projects):
- **AST pattern matching**: ast-grep (Rust binary, no CGO, 223 language support)
- **Code intelligence**: Sourcegraph (SaaS or self-hosted via Docker)
- **Clone detection**: jscpd (pure Rust, AI-ready reporters, MCP server)

---

## Sources & References

- Semgrep: [https://semgrep.dev/docs/writing-rules/pattern-syntax](https://docs.semgrep.dev/writing-rules/pattern-syntax)
- ast-grep: [https://ast-grep.github.io/guide/pattern-syntax.html](https://ast-grep.github.io/guide/pattern-syntax.html)
- CodeQL: [https://codeql.github.com/](https://codeql.github.com/)
- Sourcegraph: [https://sourcegraph.com/](https://sourcegraph.com/)
- tree-sitter-graph: [https://github.com/tree-sitter/tree-sitter-graph](https://github.com/tree-sitter/tree-sitter-graph)
- PMD CPD: [https://pmd.github.io/pmd/pmd_userdocs_cpd.html](https://pmd.github.io/pmd/pmd_userdocs_cpd.html)
- jscpd: [https://github.com/kucherenko/jscpd](https://github.com/kucherenko/jscpd)
- sqlite-vec: [https://github.com/asg017/sqlite-vec](https://github.com/asg017/sqlite-vec)
- LanceDB: [https://docs.lancedb.com/](https://docs.lancedb.com/)
- Vale: [https://vale.sh/](https://vale.sh/)
- Backscroll M1 embeddings spike: `/Users/Shared/harness/backscroll/docs/research/2026-07-embeddings-spike.md`

---

## Research Metadata

**Researcher**: Claude Code  
**Date**: 2026-07-16  
**Time spent**: ~1.5 hours (web research + primary source fetching + synthesis)  
**Artifacts**: This file (research doc) — no code artifacts or spike branch
