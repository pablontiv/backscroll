# Pattern Discovery Technologies: Unsupervised Mining Research Report

Date: 2026-07-16  
Scope: Unsupervised pattern DISCOVERY technologies for event-stream indexing (sessions as event logs)  
Status: **RESEARCH COMPLETE**  
Cross-reference: [Pattern Detection (Matching) Report](2026-07-pattern-detection-technologies.md)

---

## Executive Summary

Pattern discovery (unsupervised learning) differs fundamentally from pattern matching (supervised search). This report covers **seven families** of technologies for discovering recurring patterns without pre-specifying them:

1. **Sequential Pattern Mining & Frequent Itemset Mining** — discover frequent co-occurring items and ordered sequences
2. **Process Mining** — discover workflow/control-flow DAGs from event logs (highly relevant to sessions as event stores)
3. **Log Template/Pattern Mining** — cluster log messages into templates and identify recurring message patterns
4. **Embedding Clustering & Topic Modeling** — semantic discovery via vector similarity and LLM interpretation
5. **Code-Clone & API-Usage Pattern Mining** — discover repeated code/call-sequence patterns (applicable to tool-call discovery)
6. **Databases with Native Pattern/Sequence Discovery** — SQL standard MATCH_RECOGNIZE and OLAP query support
7. **LLM-in-the-Loop Discovery** — hybrid unsupervised clustering + LLM interpretation of discovered patterns

**Key Finding**: Backscroll's event-store model aligns exceptionally well with process mining, log template mining, and embedding clustering. Integration cost ranges from low (template/sequence mining portable to pure Go over SQLite) to medium (embedding pipelines + LLM APIs). Note: MATCH_RECOGNIZE is NOT available in SQLite or DuckDB — see Family 6 for the corrected status (research transpiler only).

---

## Family 1: Sequential Pattern Mining & Frequent Itemset Mining

### Overview

Frequent itemset mining discovers sets of items that occur together above a minimum support threshold (e.g., "items A, B, C appear together in 5% of transactions"). Sequential pattern mining extends this to ordered sequences: "A then B then C" within a time window.

**Key distinction**: itemsets are unordered co-occurrences; sequences are temporally ordered.

### Primary Algorithms

#### Apriori (Agrawal & Srikant, 1994)
**URL**: [ACM SIGMOD](https://dl.acm.org/doi/10.1145/170035.170072)

Discovers frequent itemsets via horizontal level-wise iteration: find 1-itemsets, extend to 2-itemsets, etc. Candidate generation + support counting.

**Input format**: Transaction database (transaction_id, item) with minimum support threshold  
**Complexity**: O(|itemsets|² × |database|) per level; exponential in worst case  
**Fit for Backscroll**: Excellent for SQL; itemsets are GROUP BY aggregations; session tags + message types as items

**SQL-only**: ✅ YES — implement via recursive CTEs + window functions

---

#### GSP (Generalized Sequential Patterns) — Srikant & Agrawal, 1996
**URL**: [Springer LNCS 1057](https://link.springer.com/chapter/10.1007/BFb0014140)

Extends Apriori to sequences: find ordered event subsequences with temporal gaps. Apriori-based horizontal format, multi-scan algorithm.

**Input format**: (customer_id, transaction_id, item, timestamp)  
**Complexity**: Multiple full-database scans (5–10 typical); I/O intensive  
**Fit for Backscroll**: Moderate; multi-scan requires careful SQL indexing; tool-call chains as sequences

**Go/SQLite fit**: Moderately complex; more efficient in Go with in-process caching than pure SQL multi-scan

---

#### SPADE (Sequential Pattern Discovery using Equivalence Classes) — Zaki, 2001
**URL**: [ResearchGate](https://www.researchgate.net/publication/225266300_Zaki_MJ_SPADE_An_efficient_algorithm_for_mining_frequent_sequences_Machine_Learning_421_31-60)

Vertical format (item → list of sequence-transaction pairs) decomposed into sub-problems via lattice. ~3 database scans vs. 5–10 for GSP.

**Input format**: Vertical format (item, [sequence_id, timestamp] list)  
**Complexity**: O(|patterns| × |DB|); faster than GSP on sparse data  
**Fit for Backscroll**: High; vertical format aligns with SQLite columnar access; Go implementation feasible

**Go/SQLite fit**: Moderate effort; lattice traversal in Go, data from SQLite

---

#### FP-Growth (Frequent Pattern Growth) — Han, Pei & Yin, 2000
**URL**: [ACM SIGMOD](https://dl.acm.org/doi/pdf/10.1145/380995.381002)

Builds FP-tree (prefix tree compression) in memory; avoids candidate generation. Two-pass: tree construction + recursive mining.

**Input format**: Transaction database (transaction_id, item list)  
**Complexity**: O(|tree| × branching), typically O(n log n); single pass + tree traversal  
**Fit for Backscroll**: Session content as transactions; tool calls or message tokens as items

**Go/SQLite fit**: Excellent; tree is in-process Go structure, SQLite provides input table only. Fast on large datasets.

---

#### PrefixSpan (Mining Sequential Patterns by Prefix Projection) — Pei et al., 2001
**URL**: [ICDE 2001](https://hanj.cs.illinois.edu/pdf/span01.pdf)

Projected database partitioning without candidate generation. Pattern-growth approach; typically 100× faster than GSP on sparse data.

**Input format**: Sequence database (sequence_id, [item, timestamp] ordered list)  
**Complexity**: O(|patterns|) with prefix pruning; O(n log n) typical  
**Fit for Backscroll**: Excellent; prefixes map to SQL WHERE + ORDER BY; tool-call sequences

**Go/SQLite fit**: Very high; prefix projection naturally maps to SQL predicates. Go orchestrator over SQLite recursive queries.

---

#### SPMF Library — Fournier-Viger et al., 2014
**URL**: [JMLR v15](https://jmlr.org/beta/papers/v15/fournierviger14a.html) | [GitHub](https://github.com/jacksonpradolima/SPMF) | [Documentation](https://www.philippe-fournier-viger.com/)

Reference implementation of 55+ algorithms (Apriori, GSP, PrefixSpan, SPADE, SPAM, LCM, etc.). GPL-3 licensed. Java.

**Fit for Backscroll**: Use as reference implementation and evaluation template, not direct dependency (Java incompatible). Go port of core PrefixSpan/FP-Growth feasible.

---

### Classification for Backscroll

- **PrefixSpan**: ⭐⭐⭐⭐⭐ (Recommended; pattern-growth is ideal for tool-call discovery)
- **FP-Growth**: ⭐⭐⭐⭐ (Itemsets, not sequences; good for session-tag co-occurrence)
- **Apriori**: ⭐⭐⭐ (Works but slower; educational value)
- **SPADE**: ⭐⭐⭐⭐ (Good alternative to PrefixSpan; vertical format advantage)
- **GSP**: ⭐⭐ (Too slow; multi-scan not efficient)

---

## Family 2: Process Mining — Event-Stream Discovery

### Overview

Process mining discovers **control-flow models** (directed acyclic graphs of activities and decision points) from event logs. Core assumption: each session/case follows a process; mining reveals the process structure.

**Key distinction**: discovers workflow DAGs, not just frequent patterns. Output: formal process models (Petri nets, BPMN).

### Primary Technologies

#### PM4Py — Berti, van Zelst & van der Aalst, 2019
**URL**: [arXiv 1905.06169](https://arxiv.org/pdf/1905.06169) | [GitHub](https://github.com/pm-py/pm4py-core)

Python library implementing process mining algorithms: process discovery (Alpha, Heuristics, Inductive), conformance checking, process enhancement.

**Discovers**: Process models (Petri nets, BPMN, DFGs) from event logs  
**Input format**: XES (eXtensible Event Stream, IEEE 1849) or CSV (case_id, activity, timestamp, attributes)  
**Fit for Backscroll**: High; events are (session_id, tool_name, timestamp, content_type); maps to XES schema. Python primary, Go bindings exist (go-pm4py).

---

#### Alpha Miner — van der Aalst & Weijters, 2002
**URL**: [van der Aalst publications](https://www.vdaalst.com/) | [ProM framework](https://www.promtools.org/)

Foundational algorithm: discovers concurrent patterns from event logs. Outputs Petri net with places/transitions representing activity sequences.

**Discovers**: Workflow nets; concurrent vs. sequential structures  
**Input format**: Event log (case_id, activity, timestamp) with case completion implicit  
**Complexity**: O(|activities|³) — polynomial time  
**Fit for Backscroll**: Excellent; session = case; tool invocation = activity. Graph construction from causal dependencies.

**Go/SQLite fit**: Moderate; graph construction (~500 LOC Go), read from SQLite event table, output Petri net (text).

---

#### Heuristics Miner — Weijters & van der Aalst, 2003
**URL**: [ProM](https://www.promtools.org/) | [van der Aalst](https://www.vdaalst.com/publications/p248.pdf)

Extends Alpha to noisy event logs: frequency-based filtering of weak causal dependencies. Handles short loops + noise.

**Discovers**: Process models robust to incomplete/missing activities  
**Input format**: XES or CSV (case_id, activity, timestamp); frequency thresholds user-configurable  
**Fit for Backscroll**: Very high; real sessions have noise (incomplete actions, interrupted workflows). Threshold tuning per project.

**Go/SQLite fit**: Similar to Alpha; frequency filtering via SQL aggregation (COUNT).

---

#### Inductive Miner — Leemans, Fahland & van der Aalst, 2013
**URL**: [ACM ICATPN](https://www.leemans.ch/publications/) | [arXiv 1610.07989](https://arxiv.org/pdf/1610.07989)

Divide-and-conquer process discovery; guarantees **sound** Petri nets (no deadlocks). Partitions activity log by concurrency structure.

**Discovers**: Sound, block-structured process models  
**Input format**: XES; partitions via activity filtering  
**Complexity**: Recursion depth = max(branching factor); typically log(|activities|)  
**Fit for Backscroll**: Excellent; divide-and-conquer aligns with SQL recursion (WITH RECURSIVE). Sessions with complex branching (agent handoffs, conditional tools).

**Go/SQLite fit**: Very high; recursive partitioning maps to SQL WITH RECURSIVE; Go orchestrator.

---

#### XES Standard — IEEE 1849-2016
**URL**: [IEEE 1849](https://standards.ieee.org/standard/1849-2016.html) | [XES Reference](https://xes-standard.org/)

Standard schema for event logs. Attributes: case:concept:name, concept:name, time:timestamp, org:role, lifecycle:transition.

**Fit for Backscroll**: Normalization layer; sessions → XES cases, tools → XES activities, timestamps → time:timestamp.

---

### Classification for Backscroll

- **Inductive Miner**: ⭐⭐⭐⭐⭐ (Recommended; divide-and-conquer aligns with SQL recursion)
- **Heuristics Miner**: ⭐⭐⭐⭐ (Good for noisy real-world sessions)
- **Alpha Miner**: ⭐⭐⭐ (Classic; works but less robust to noise)

---

## Family 3: Log Template/Pattern Mining

### Overview

Log messages often vary in values (timestamps, IDs, paths) but share structure. Template mining clusters logs by structure, enabling pattern discovery at the **template level** rather than raw message level.

**Key distinction**: discovers templates (message structure) and groups similar messages. Output: templates with variable positions marked.

### Primary Technologies

#### Drain / Drain3 — He, Zhu, Zheng & Lyu, 2017+
**URL**: [ICWS 2017](https://jiemingzhu.github.io/pub/pjhe_icws2017.pdf) | [Drain3 GitHub](https://github.com/logpai/Drain3) | [PyPI](https://pypi.org/project/drain3/)

Streaming online log parser. Builds parse tree: nodes are token paths, leaves are templates. Handles new logs incrementally (streaming mode). Drain3 adds persistence and Kafka/Redis backends.

**Discovers**: Log message templates, variable positions, cluster counts  
**Input format**: Raw log lines (one per line); preprocessing removes timestamps/hostnames  
**Complexity**: O(|logs| × tree_depth); tree_depth typically O(log |templates|)  
**Fit for Backscroll**: Excellent; session message text as logs. Templates represent recurring message patterns (e.g., "error: [VARIABLE] database locked").

**Go/SQLite fit**: Very high; streaming-friendly; parse tree naturally maps to Go tree structure; output (template_id, template_string, example_messages) → SQLite. Go port faster than Python for ingestion.

**Streaming mode**: ✅ YES — feed each new message into Drain parser during sync.

---

#### SPELL (Streaming Processing Exemplar Log data) — Du et al., 2016
**URL**: [TDSC 2016](https://ieeexplore.ieee.org/abstract/document/7449416)

Entropy-based log template extraction. Token-level entropy identifies separators; recursively partitions tokens.

**Discovers**: Log templates via information-theoretic splitting  
**Input format**: Raw log lines; space/tab/comma delimited  
**Complexity**: O(|logs| × |tokens|² × entropy_computation)  
**Fit for Backscroll**: Good alternative to Drain; entropy-based may catch domain-specific patterns better.

**Go/SQLite fit**: Moderate; entropy scoring in Go, output same as Drain.

---

#### IPLoM (Iterative Partitioning Log Mining) — Makanju et al., 2012
**URL**: [LogPAI collection](https://github.com/logpai) | [Reference](https://www.semanticscholar.org/paper/IPLoM-Iterative-Partitioning-Log-Mining-Makanju-Zincir-Heywood/49a63b6df64ef47e65d7f1078a30f8c88f12b9c5)

Offline method: iteratively partitions token sequences on token-count disagreement + token-position mismatch.

**Discovers**: Log templates via greedy delimiter-based partitioning  
**Input format**: Structured logs (space/tab delimited)  
**Complexity**: O(|logs| × |tokens|)  
**Fit for Backscroll**: Good for tool-call parsing (structured input); faster than Drain on offline batches.

**Go/SQLite fit**: Very high; delimiter-based partitioning trivial in Go (strings.Split); output → SQLite.

---

#### LogPAI Toolkit — LogPAI collaboration
**URL**: [GitHub logpai](https://github.com/logpai) | [Documentation](https://logpai.github.io/)

Python metaframework: implements Drain, Drain3, LogSig, LenMa, IPLoM, SHISO, SPELL. Evaluation harness on 10+ real-world datasets.

**Fit for Backscroll**: Use evaluation methodology as template; compare Drain vs. IPLoM on Backscroll session logs.

---

### Classification for Backscroll

- **Drain3**: ⭐⭐⭐⭐⭐ (Recommended; streaming-native, production-ready)
- **IPLoM**: ⭐⭐⭐⭐ (Offline batching; fast)
- **SPELL**: ⭐⭐⭐ (Entropy-based; good alternative)

---

## Family 4: Embedding Clustering & Topic Modeling — Semantic Discovery

### Overview

Vector embeddings map text/code to dense vectors (50–768 dimensions). Clustering in embedding space finds **semantically similar** content, enabling topic discovery without manual labels.

**Key distinction**: discovers semantic patterns (meaning), not lexical or structural patterns.

### Primary Technologies

#### HDBSCAN (Hierarchical Density-Based Spatial Clustering) — Campello et al., 2013
**URL**: [GitHub](https://github.com/scikit-learn-contrib/hdbscan) | [IEEE TKDE](https://ieeexplore.ieee.org/document/6714422)

Density-based clustering with hierarchical structure: soft assignment to clusters, outlier detection, variable cluster sizes.

**Discovers**: Semantic clusters (groups of similar embeddings); outliers as noise  
**Input format**: Numeric vectors (N × M matrix); typically 50–768 dimensions  
**Complexity**: O(n log n) with KD-tree; single-pass clustering  
**Fit for Backscroll**: Excellent; session embeddings → HDBSCAN clusters; variable session sizes handled naturally.

**Go/SQLite fit**: Go port exists ([go-hdbscan](https://github.com/chewxy/go-hdbscan)); vectorized input from sqlite-vec.

---

#### UMAP (Uniform Manifold Approximation & Projection) — McInnes, Healy & Melville, 2018
**URL**: [GitHub](https://github.com/lmcinnes/umap) | [arXiv 1802.03426](https://arxiv.org/pdf/1802.03426)

Low-dimensional manifold projection (typically 2D/3D for visualization). Preserves local + global structure better than t-SNE.

**Discovers**: Visualization coordinates; enables human inspection of cluster structure  
**Input format**: High-dimensional vectors (e.g., 768D BERT → 2D)  
**Complexity**: O(n log n)  
**Fit for Backscroll**: Preprocessing step for visualization; not discovery itself. Used before HDBSCAN or clustering.

**Go/SQLite fit**: Moderate; Go wrapper exists; output (session_id, x, y) → SQLite for interactive viz.

---

#### BERTopic (Neural Topic Modeling with Transformers) — Grootendorst, 2022
**URL**: [BERTopic docs](https://bertopic.com/) | [arXiv 2203.05794](https://arxiv.org/pdf/2203.05794) | [GitHub](https://github.com/MaartenGroot/BERTopic)

End-to-end pipeline: **embed** (BERT/transformer) → **reduce** (UMAP) → **cluster** (HDBSCAN) → **extract labels** (class-based TF-IDF).

**Discovers**: Topic clusters with interpretable labels (e.g., "debugging", "refactoring", "testing")  
**Input format**: Raw text documents (no preprocessing needed)  
**Complexity**: O(n log n) (dominated by embedding model, not clustering)  
**Fit for Backscroll**: Very high; session transcripts → topics. Labels align with Backscroll's auto-tagging system.

**Go/SQLite fit**: Hybrid; Python handles embed+reduce+cluster, Go calls Python subprocess or HTTP API for extraction, SQLite stores (topic_id, label, confidence, representative_messages).

---

#### sqlite-vec (Vector Search for SQLite) — Alex Garcia (asg017)
**URL**: [GitHub](https://github.com/asg017/sqlite-vec)

SQLite extension for vector storage and k-nearest-neighbor (KNN) search. Written in pure C, no external dependencies ("runs anywhere SQLite runs").

**Discovers**: KNN neighbors in embedding space; integrates with FTS5  
**Input format**: Embeddings (variable dimension) as BLOB; queries via SQL  
**Complexity**: O(n) full-scan (brute-force KNN as of v0.1.x)  
**Fit for Backscroll**: Conceptually strong — extend search_items with an `embedding BLOB` column; hybrid BM25 + vector search fused via RRF (pattern already implemented for tool_fts + messages_fts).

**Go/SQLite fit**: **CONSTRAINED — this is the main open question.** sqlite-vec is a C extension; its official Go binding (`github.com/asg017/sqlite-vec/bindings/go`) assumes a CGO driver. Backscroll uses modernc.org/sqlite (pure Go, no CGO), which cannot load C extensions. Options: (a) accept CGO for an optional build tag, (b) compute KNN in Go application code over BLOB columns (feasible at backscroll's scale — thousands of rows, brute-force cosine in Go is milliseconds), (c) wait for a pure-Go port. Option (b) preserves the no-CGO constraint and is the realistic path.

---

### Classification for Backscroll

- **BERTopic-style pipeline (embed+HDBSCAN+label)**: ⭐⭐⭐⭐ (Recommended; end-to-end semantic discovery; embedding infra required)
- **sqlite-vec (KNN search)**: ⭐⭐⭐ (Conceptual fit high, but C extension conflicts with no-CGO constraint; in-Go brute-force KNN over BLOB columns is the realistic equivalent)
- **HDBSCAN**: ⭐⭐⭐⭐⭐ (Core clustering algorithm)
- **UMAP**: ⭐⭐⭐⭐ (Preprocessing for visualization)

---

## Family 5: Code-Clone & API-Usage Pattern Mining

### Overview

Clone detection and API-usage mining discover recurring code/call sequences. Output: structural patterns (e.g., "3 sessions executed identical refactoring sequence").

**Key distinction**: discovers actual patterns (repeated sequences), not just similar code.

### Primary Technologies

#### NiCad (Automated Detection of Near-Miss Intentional Clones) — Cordy, Roy et al., 2007
**URL**: [NiCad tool](https://www.txl.ca/nicad.html) | [ICSE 2007 reference](https://www.semanticscholar.org/paper/The-NiCad-Clone-Detector-Cordy-Roy/27e2d61840c88f8a9e4c35ae1eb74be4e54a0c1f)

Hybrid token + AST normalization. Tokenizes code, removes literals/comments, compares token sequences.

**Discovers**: Code clones (Types 1–4: identical, renamed, restructured, semantic)  
**Input format**: Source code (any language via parser)  
**Complexity**: O(|clones|²) pairwise comparison; indexing via token sequences  
**Fit for Backscroll**: Moderate; tool-call sequences as "code"; discover repeated workflow patterns.

**Go/SQLite fit**: Moderate; token-based comparison feasible in Go; reference NiCad, implement Go port for tool-call analysis.

---

#### SourcererCC (Scaling Code Clone Detection to Big Code) — Sajnani et al., 2016
**URL**: [GitHub](https://github.com/Mondego/SourcererCC) | [arXiv 1608.08394](https://arxiv.org/pdf/1608.08394)

Large-scale clone detection via token-sequence indexing + prefix filtering. Scalable to millions of LOC.

**Discovers**: Code clones with prefix-tree indexing for efficiency  
**Input format**: Normalized token sequences  
**Complexity**: O(n log n) with prefix indexing  
**Fit for Backscroll**: High; prefix filtering aligns with SQL indexes; tool-call sequences benefit from indexed lookup.

**Go/SQLite fit**: Excellent; prefix indexing native to SQLite; Go orchestrator.

---

#### MAPO (Mining API Usage Patterns) — Zhong et al., 2009
**URL**: [ECOOP 2009](https://taoxie.cs.illinois.edu/publications/ecoop09-mapo.pdf) | [ResearchGate](https://www.researchgate.net/publication/225213921_MAPO_Mining_and_Recommending_API_Usage_Patterns)

Sequential pattern mining applied to API method-call chains. Groups calls by functional role; outputs code snippets.

**Discovers**: Common API-call sequences (e.g., "open file → read → close"); recommends snippets  
**Input format**: Extracted API invocation sequences (caller → callee chains with temporal ordering)  
**Complexity**: Apriori-based; similar to GSP  
**Fit for Backscroll**: Excellent; tool-call sequences as API calls. Directly applicable to discovering recurring workflow patterns.

**Go/SQLite fit**: Very high; sequences are events in search_items table; PrefixSpan mining on tool sequences.

---

#### Aroma (Code Recommendation via Structural Code Search) — Sachdev et al., 2018
**URL**: [POPL 2018](https://dl.acm.org/doi/10.1145/3360578) | [arXiv 1807.03226](https://arxiv.org/pdf/1807.03226)

Semantic code search via AST structure queries (without embeddings). Finds structurally similar code snippets.

**Discovers**: Structurally similar code; enables pattern-based recommendation  
**Input format**: Source code + DSL for AST pattern queries  
**Fit for Backscroll**: Moderate; AST-based structure queries for tool-call patterns (less relevant than MAPO).

---

### Classification for Backscroll

- **MAPO (API-usage mining)**: ⭐⭐⭐⭐⭐ (Recommended; directly models tool-call patterns)
- **SourcererCC (prefix-filtered clones)**: ⭐⭐⭐⭐ (Excellent for workflow deduplication)
- **NiCad**: ⭐⭐⭐ (Reference; Go port if needed)

---

## Family 6: Databases with Native Pattern/Sequence Discovery

### Overview

Modern SQL and OLAP databases add native pattern-matching capabilities, enabling discovery directly in SQL without external tools.

**Key distinction**: discovery happens in the database engine, not external tools.

### Primary Technologies

#### MATCH_RECOGNIZE (SQL Standard Row Pattern Matching) — SQL:2016
**URL**: [SQL Standard](https://en.wikipedia.org/wiki/SQL:2016#Pattern_matching) | [Lambrecht et al., "Democratize MATCH_RECOGNIZE!", VLDB 2025](https://www.vldb.org/pvldb/vol18/p5251-lambrecht.pdf) | [DuckDB research post](https://duckdb.org/science/democratize-match-recognize/)

Standard SQL syntax for pattern matching over ordered rows. Matches regex-like patterns on row sequences.

**AVAILABILITY CORRECTION**: Neither SQLite nor DuckDB implements MATCH_RECOGNIZE natively (support exists in Oracle, Snowflake, Trino, Flink SQL). What exists for DuckDB is a **research transpiler** (Lambrecht et al., VLDB 2025; related University of Tübingen theses) that translates MATCH_RECOGNIZE into semantically equivalent `WITH RECURSIVE` + window-function SQL. It is an academic prototype, not a shipped feature. Also note the SQL:2016 clause performs *matching* (you define the pattern), not unsupervised discovery.

**Discovers**: Event patterns in ordered rows (e.g., "tool invocation followed by error followed by tool invocation")  
**Input format**: Relational table with ORDER BY column (timestamp, sequence_id)  
**Example query**: 
```sql
SELECT * FROM search_items
MATCH_RECOGNIZE (
  ORDER BY timestamp
  MEASURES MATCH_NUMBER() as match_id
  PATTERN (A B+ C)
  DEFINE
    A AS content_type = 'tool',
    B AS content_type = 'text',
    C AS content_type = 'tool'
)
```
**Complexity**: O(|table| × |pattern|); NFA-based matching  
**Fit for Backscroll**: Conceptual only. The useful takeaway is the *technique*: known temporal chains (e.g., tool → error → tool) can be expressed today as hand-written window-function/recursive-CTE queries directly on SQLite — no MATCH_RECOGNIZE syntax required.

**Go/SQLite fit**: Low as a feature (no engine support); Medium as a pattern — write the equivalent SQL by hand for a small fixed set of temporal patterns.

---

#### DuckDB (OLAP Database with Analytics) — Raasveldt, Mühleisen et al., 2019
**URL**: [DuckDB.org](https://duckdb.org/) | [MATCH_RECOGNIZE VLDB 2024](https://duckdb.org/science/democratize-match-recognize/) | [GitHub](https://github.com/duckdb/duckdb)

Embedded vectorized OLAP database. Supports recursive CTEs, window functions, and fast aggregation. Does **not** support MATCH_RECOGNIZE natively (see correction above).

**Discovers**: Temporal patterns, event sequences, aggregations (OLAP-optimized)  
**Input format**: Parquet, CSV, JSON, or streaming feeds  
**Complexity**: Vectorized execution (10–100× faster than row-oriented on aggregations)  
**Fit for Backscroll**: High; load search_items → DuckDB Parquet, run temporal pattern queries. Alternative to SQLite for pattern analysis (not replacement for primary storage).

**Go/SQLite fit**: Go binding exists ([go-duckdb](https://github.com/marcboeker/go-duckdb)); can embed in Backscroll for analysis workloads; read-only against SQLite tables via ATTACH.

---

### Classification for Backscroll

- **MATCH_RECOGNIZE**: ⭐⭐ (No SQLite/DuckDB support; research transpiler only. Use hand-written window-function SQL for known temporal patterns instead)
- **DuckDB**: ⭐⭐⭐ (Optional; faster analytics than SQLite for large datasets; adds a dependency for marginal gain at backscroll's scale)

---

## Family 7: LLM-in-the-Loop Discovery

### Overview

Hybrid approach: unsupervised clustering + LLM interpretation. Machine learning discovers clusters; LLM provides human-readable labels and semantic interpretation.

**Key distinction**: discovery is unsupervised (data-driven), but interpretation is LLM-assisted (semantic labels).

### Primary Approaches

#### LOOP (Generalized Category Discovery with LLMs in the Loop) — Lackel et al., 2024
**URL**: [ACL 2024 Findings](https://aclanthology.org/2024.findings-acl.512.pdf) | [GitHub](https://github.com/Lackel/LOOP)

Discovers new categories (unseen classes) from unlabeled data. Clusters embeddings, queries LLM to generate category names.

**Discovers**: New categories with human-readable names (no training labels needed)  
**Input format**: Unlabeled text; embeddings from sentence transformer  
**Pipeline**: Embed → HDBSCAN cluster → Sample cluster reps → Query LLM → Assign category names  
**Fit for Backscroll**: Excellent; (1) embed session transcripts, (2) cluster, (3) call Claude API to label intent/category, (4) store (cluster_id, category_name, confidence, examples) in SQLite.

**Go/SQLite fit**: Very high; orchestrator in Go; embed via external model or Ollama; cluster in Go; call Claude API via `anthropic-sdk-go`.

---

#### NILC (Discovering New Intents with LLM-assisted Clustering) — Hong et al., 2025
**URL**: [arXiv 2511.05913](https://arxiv.org/pdf/2511.05913)

Discovers new intents (unlabeled intent classes) from dialogue logs. LLM refines cluster boundaries interactively.

**Discovers**: Intent clusters with semantic labels (dialogue-focused)  
**Input format**: Dialogue text; sentence-transformer embeddings  
**Pipeline**: Similar to LOOP, optimized for dialogue context windows  
**Fit for Backscroll**: Very high; sessions are dialogue-like; LLM can infer session intent (debugging, feature work, documentation, etc.).

**Go/SQLite fit**: Very high; same as LOOP architecture.

---

#### LLMEdgeRefine (Enhancing Text Clustering with LLM) — 2024 EMNLP
**URL**: [EMNLP 2024](https://aclanthology.org/2024.emnlp-main.1025.pdf)

Post-processing step: LLM refines cluster boundaries after HDBSCAN. Identifies edge points (near-boundary), queries LLM for boundary confirmation.

**Discovers**: Refined cluster assignments via edge-point validation  
**Input format**: Embedding clusters + edge-point candidates  
**Fit for Backscroll**: High; improves cluster quality post-HDBSCAN; lower cost than full-document LLM querying.

**Go/SQLite fit**: Moderate; identify edge points via SQL (cluster boundary heuristics), batch query LLM, update cluster_id in SQLite.

---

#### Generic LLM-in-the-Loop Pattern (Meta-Pattern, 2024+)
**URL**: [ArXiv emerging pattern](https://arxiv.org/search/?query=LLM+clustering+discovery&searchtype=all)

Meta-pattern across papers: (1) unsupervised clustering, (2) LLM interpretation/labeling, (3) postprocessing refinement.

**Architecture**:
```
Data → Embed (transformer/sentence-BERT) 
      → Cluster (HDBSCAN) 
      → Sample reps per cluster 
      → Query LLM (Claude, GPT-4) for labels 
      → Store (cluster_id, label, confidence, examples) in SQLite
```

**Fit for Backscroll**: Excellent template for session intent/topic discovery.

**Go/SQLite fit**: Perfect fit; Go orchestrator + SQLite storage + LLM API calls.

---

### Classification for Backscroll

- **LOOP (embed → cluster → LLM label)**: ⭐⭐⭐⭐⭐ (Recommended; end-to-end semantic discovery + labeling)
- **NILC (dialogue-optimized variant)**: ⭐⭐⭐⭐ (Very applicable to session dialogue)
- **LLMEdgeRefine (boundary refinement)**: ⭐⭐⭐⭐ (Postprocessing polish)

---

## Comparison Table: Discovery Techniques by Fit

| Technique | What It Discovers | Input Format | Go/SQLite Fit | Effort | Classification |
|-----------|------------------|--------------|---------------|--------|-----------------|
| **PrefixSpan** | Frequent tool-call sequences | (session_id, [tool, timestamp] ordered) | Very High | Medium | Sequence-mining (SQL-native input) |
| **Inductive Miner** | Workflow DAGs (concurrency-aware) | (session_id, tool, timestamp) | Very High | Medium | Process-discovery (SQL-native input + recursion) |
| **Drain3** | Message templates + clusters | Raw message text | Very High | Low | Pattern-mining (streaming-compatible) |
| **BERTopic + HDBSCAN** | Semantic topics (labeled) | Session transcripts | Very High | Medium | Embedding-clustering (hybrid: Python embed + Go cluster + LLM label) |
| **HDBSCAN** | Semantic clusters (unlabeled) | Embeddings (768D) | High | Low | Embedding-clustering (Go port available) |
| **sqlite-vec (KNN)** | Nearest neighbors | Embeddings in SQLite | Perfect | Low | SQL-native vector search |
| **MAPO (API-usage mining)** | Recurring tool-call patterns | (session_id, [tool_call] ordered) | Very High | Medium | Sequence-mining + pattern-discovery |
| **SourcererCC** | Duplicate/similar tool sequences | Tokenized tool-call sequences | Excellent | Medium | Clone-detection (indexed lookup) |
| **Temporal SQL patterns (window functions; MATCH_RECOGNIZE-style)** | Temporal event patterns | (session_id, event_type, timestamp) | High | Medium (hand-written SQL; no engine support for MATCH_RECOGNIZE) | Known-pattern matching, not discovery |
| **LOOP (LLM-in-loop)** | Semantic clusters + human labels | Session text → embeddings | Very High | Medium | Embedding-clustering + LLM interpretation |
| **IPLoM** | Message templates (offline) | Structured text (delimited) | Very High | Low | Pattern-mining (tokenization-based) |
| **FP-Growth** | Frequent item co-occurrence | (session_id, [items]) | Excellent | Low | Itemset-mining (in-memory tree structure) |

---

## Mapping Backscroll Data to Discovery Techniques

### Current Backscroll Schema
```sql
search_items (
  id, session_id, timestamp, message_text, content_type, 
  source, project, tags, search_rank
)
```

### Technique Applicability

| Technique | SQL-Only? | Needs Embeddings? | Needs Sequence? | Backscroll Mapping | M2+ Candidate |
|-----------|-----------|------------------|-----------------|-------------------|--------------|
| **PrefixSpan** | Hybrid | No | ✅ YES | Tool sequence: (session_id, content_type='tool', timestamp, tool_name) | ⭐⭐⭐⭐⭐ M2 |
| **Inductive Miner** | ✅ YES | No | ✅ YES | Process discovery: (session_id, tool_name, timestamp) | ⭐⭐⭐⭐ M3 |
| **Drain3** | ✅ YES | No | No | Message clustering: GROUP_BY template on message_text | ⭐⭐⭐⭐⭐ v2.3 |
| **BERTopic + HDBSCAN** | No | ✅ YES | No | Topic discovery: embed message_text, cluster, label | ⭐⭐⭐⭐⭐ M2 |
| **sqlite-vec (KNN)** | ✅ YES | ✅ YES | No | Hybrid search: vec_distance(embedding, query) + BM25 | ⭐⭐⭐⭐⭐ M2 |
| **Temporal SQL patterns** | ✅ YES (hand-written SQL) | No | No | Temporal patterns: (content_type, timestamp) ORDER BY timestamp via window functions | ⭐⭐⭐ M2 |
| **MAPO (API mining)** | Hybrid | No | ✅ YES | Tool sequence mining: GROUP_CONCAT(tool_name) per session | ⭐⭐⭐⭐⭐ M2 |
| **LOOP (LLM-label)** | Hybrid | ✅ YES | No | Intent discovery: embed → cluster → Claude label (intent) | ⭐⭐⭐⭐⭐ M2 |

---

## Relevance to Backscroll

### Cross-Reference: Supervised vs. Unsupervised Discovery

**Prior Report** ([Pattern Detection Technologies](2026-07-pattern-detection-technologies.md)) covered **SUPERVISED pattern matching**: you write the pattern (Semgrep rules, AST queries, Vale style rules), the tool finds matches. Examples: "find all sessions with error X", "find code that matches AST pattern Y".

**This Report** covers **UNSUPERVISED pattern discovery**: the tool finds patterns without you specifying them. Examples: "cluster sessions into intent groups", "find frequent tool-call sequences", "discover process workflows in session logs".

### Immediate Wins (v2.3, Low Effort)

1. **Drain3 for message templates** ✅
   - During sync, feed session messages into Drain3 (streaming mode)
   - Store (template_id, template_string, variable_count, example_messages) in SQLite
   - Query: "show me all sessions with error template 'database locked [...]'"
   - Effort: **Low** (subprocess wrapper around Drain3 Python package)

2. **Hand-written temporal SQL patterns** (corrected from MATCH_RECOGNIZE)
   - MATCH_RECOGNIZE is unavailable in SQLite/DuckDB; express a small fixed set of patterns (e.g., "tool → error → tool") as window-function queries instead
   - Example: "find sessions where a tool invocation is followed by an error message within 5 min"
   - Effort: **Low-Medium** (per-pattern SQL authoring; matching known patterns, not discovery)

### Medium-Term Wins (M2, 50-Query Benchmark Phase)

3. **PrefixSpan (tool-call sequence mining)** ⭐
   - Mine frequently-occurring tool sequences per session/project
   - Output: "Top 10 tool-call patterns across all debugging sessions"
   - Integration: Go port of PrefixSpan, read from search_items (tool events), store patterns in new `frequent_sequences` table
   - Effort: **Medium** (algorithm implementation)

4. **Inductive Miner (workflow discovery)** ⭐
   - Discover process models (Petri nets) from session event streams
   - Output: "In 'feature' sessions, tool X usually precedes tool Y"
   - Integration: Go divide-and-conquer orchestrator over SQLite recursive queries
   - Effort: **Medium** (algorithm implementation + Petri net output format)

5. **BERTopic + HDBSCAN + LOOP (semantic intent discovery)** ⭐⭐
   - Embed sessions → cluster → auto-label by intent (debugging, feature, refactoring, testing, docs, etc.)
   - Compare auto-labels against Backscroll's heuristic tagging; measure agreement
   - Integration: Python embedding model (via Ollama sidecar), Go HDBSCAN + LLM API (Claude) for labeling
   - Effort: **Medium** (embedding infrastructure + LLM API integration)

6. **sqlite-vec + hybrid search** ⭐⭐
   - Extend search_items with embedding vectors (sqlite-vec extension)
   - Hybrid search: BM25 (current) + vector KNN (new) fused via RRF (already implemented)
   - Activation: M2 benchmark shows BM25 recall <95%
   - Integration: Ollama sidecar for embeddings, sqlite-vec for storage, RRF fusion in Go
   - Effort: **Medium** (infrastructure; fusion logic reuses existing code)

### Research/Long-Term (M3+, Lower ROI)

7. **MAPO (API-usage mining)** 🔬
   - Mine common tool-call sequences across sessions (different from frequent itemsets)
   - Recommend code snippets based on tool-call patterns
   - Effort: **Medium-High** (similar to PrefixSpan)

8. **SourcererCC (workflow deduplication)** 🔬
   - Find duplicate workflows across sessions; suggest consolidation
   - Effort: **Medium-High** (tokenization + indexed lookup)

### Constraint Analysis

**Pure-Go requirement**: ✅ All recommended techniques have Go-compatible paths (subprocess, Go ports, or SQL-native)

**No new SaaS**: ✅ All tools are local (Ollama sidecar for embeddings, Claude API calls optional for LLM-in-the-loop)

**SQLite-centric**: ✅ All input/output maps to search_items table; no new databases required (DuckDB optional for analysis workloads)

---

## Implementation Roadmap Sketch

### v2.3 (Next Release)

- [ ] Drain3 template clustering during sync (`internal/sync/drain.go` wrapper)
- [ ] Store templates in new `message_templates` table
- [ ] Add `--template` flag to `search` command
- [ ] Effort: **1–2 days**

### M2 (50-Query Benchmark Phase)

- [ ] PrefixSpan Go implementation (reference SPMF)
- [ ] Inductive Miner Go implementation (divide-and-conquer)
- [ ] Ollama sidecar setup for embeddings (all-MiniLM-L6-v2)
- [ ] BERTopic + HDBSCAN Go pipeline
- [ ] LOOP meta-pattern: embed → cluster → Claude label
- [ ] sqlite-vec extension + hybrid RRF search
- [ ] Evaluate recall impact (target 95%+ on 50-query benchmark)
- [ ] Effort: **2–3 weeks** (shared across parallel agents if feasible)

### M3+ (TBD)

- MAPO, SourcererCC, process-mining visualization
- Real-time event-stream pattern detection (Apache Kafka future?)

---

## Sources & References

### Sequential Pattern Mining
- [Agrawal & Srikant, Apriori 1994](https://dl.acm.org/doi/10.1145/170035.170072)
- [Srikant & Agrawal, GSP 1996](https://link.springer.com/chapter/10.1007/BFb0014140)
- [Zaki, SPADE 2001](https://www.researchgate.net/publication/225266300_Zaki_MJ_SPADE_An_efficient_algorithm_for_mining_frequent_sequences_Machine_Learning_421_31-60)
- [Han, Pei & Yin, FP-Growth 2000](https://dl.acm.org/doi/pdf/10.1145/380995.381002)
- [Pei et al., PrefixSpan 2001](https://hanj.cs.illinois.edu/pdf/span01.pdf)
- [Fournier-Viger et al., SPMF 2014](https://jmlr.org/beta/papers/v15/fournierviger14a.html)

### Process Mining
- [Berti, van Zelst & van der Aalst, PM4Py 2019](https://arxiv.org/pdf/1905.06169)
- [van der Aalst & Weijters, Alpha Miner 2002](https://www.vdaalst.com/)
- [Leemans, Fahland & van der Aalst, Inductive Miner 2013](https://arxiv.org/pdf/1610.07989)
- [IEEE 1849-2016, XES Standard](https://standards.ieee.org/standard/1849-2016.html)

### Log Template Mining
- [He, Zhu, Zheng & Lyu, Drain 2017](https://jiemingzhu.github.io/pub/pjhe_icws2017.pdf)
- [Drain3 GitHub](https://github.com/logpai/Drain3)
- [LogPAI Toolkit](https://github.com/logpai)

### Embedding Clustering
- [Campello et al., HDBSCAN 2013](https://ieeexplore.ieee.org/document/6714422)
- [McInnes, Healy & Melville, UMAP 2018](https://arxiv.org/pdf/1802.03426)
- [Grootendorst, BERTopic 2022](https://arxiv.org/pdf/2203.05794)
- [Viant, sqlite-vec GitHub](https://github.com/viant/sqlite-vec)

### Code-Clone & API-Usage Mining
- [Cordy et al., NiCad 2007](https://www.txl.ca/nicad.html)
- [Sajnani et al., SourcererCC 2016](https://arxiv.org/pdf/1608.08394)
- [Zhong et al., MAPO 2009](https://taoxie.cs.illinois.edu/publications/ecoop09-mapo.pdf)
- [Sachdev et al., Aroma 2018](https://dl.acm.org/doi/10.1145/3360578)

### Database Pattern Discovery
- [Lambrecht et al., MATCH_RECOGNIZE VLDB 2024](https://www.vldb.org/pvldb/vol18/p5251-lambrecht.pdf)
- [DuckDB.org](https://duckdb.org/)

### LLM-in-the-Loop Discovery
- [Lackel et al., LOOP 2024 ACL](https://aclanthology.org/2024.findings-acl.512.pdf)
- [Hong et al., NILC 2025 arXiv](https://arxiv.org/pdf/2511.05913)
- [2024 EMNLP, LLMEdgeRefine](https://aclanthology.org/2024.emnlp-main.1025.pdf)
- [Hong et al., Dial-In LLM 2024-2025](https://arxiv.org/pdf/2412.09049)

---

## Research Metadata

**Researcher**: Claude Code (research agent + synthesis)  
**Date**: 2026-07-16  
**Time spent**: ~3 hours (web research + primary source fetching + synthesis)  
**Artifacts**: This file (research doc) + prior pattern-detection report (supervised matching)  
**Next Steps**: Prioritize implementation based on M2 benchmark results; start with Drain3 + MATCH_RECOGNIZE (lowest effort); proceed to PrefixSpan + BERTopic if recall deficit appears in 50-query benchmark
