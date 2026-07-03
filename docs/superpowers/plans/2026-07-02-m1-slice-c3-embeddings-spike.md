# M1 Slice C3 — Embeddings Spike Implementation Plan

Date: 2026-07-02  
Slice: C3 — Embeddings spike (experimental, time-boxed 1 day)  
Branch: `spike/embeddings-eval` (do NOT merge to main except decision report)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

## Goal

Answer: **is a real, local, pure-Go embedding provider feasible within this spike's timebox?**

If yes, measure impact on recall@5 and latency vs BM25-only. If no, document the blocker and defer/delete.
Ship decision report (docs/research/2026-07-embeddings-spike.md) with measured metrics and verdict.

## Corrected Premise

O10 hybrid search is **NOT dormant** — HybridSearch + RRF + migration v3 + CLI flags shipped on 2026-06-22
(commit 78ab780) with safe BM25 fallback.

**What is actually missing:**
1. Real embedding provider (OnnxProvider is a stub returning `ErrOnnxNotAvailable`)
2. Sync-time vector population (T036 never implemented)
3. Measurement of real-provider impact

**This spike's scope:** research and evaluate REAL local pure-Go embedding providers. If one is feasible
in the timebox, measure hybrid search impact. If not feasible, document blockers and verdicts defaults to
defer/delete. **Mock measurement proves nothing**; the decision must be grounded in feasibility assessment + real provider measurement.

## Architecture

**Hybrid search (already complete, waiting for real embeddings):**
1. Query text → EmbeddingProvider.Embed() → query vector (384-dim)
2. BM25 search → lexical ranking list
3. Vector search → cosine similarity (linear-scan over `chunks.embedding`)
4. RRF fusion (k=60) → merged ranking
5. Results returned with RRF score

**Current state:**
- `HybridSearch()` wired to `search.go` ✅
- `--similarity-threshold` and `--lexical-only` flags exist ✅
- `EmbeddingProvider` interface + `MockEmbeddingProvider` exist ✅
- `OnnxProvider` is stub; **evaluation required**: gonnx, ollama sidecar, others ⚠️
- RRF + vector search fully implemented ✅
- Schema v2/v3 ready ✅
- Eval-set reuses A2A3 infrastructure (TOML format, runner script) ✅

**Decision verdicts:**
- **Activate**: real provider feasible, recall@5 hybrid ≥ baseline, p95 latency acceptable
- **Defer**: real provider not feasible in timebox (blockers documented), OR gains insufficient
- **Delete**: pure-Go constraint impossible AND activation burden high

## Tech Stack

- **Go 1.21+** (stdlib + go-toml, existing dependency)
- **Eval infrastructure**: A2A3 eval-set (TOML format), runner script (bash)
- **Provider candidates to research**: gonnx (pure-Go ONNX), ollama (sidecar), others
- **Test harness**: Reuse A2A3 runner; add spike-specific provider-injection wrapper
- **Metrics**: recall@5, p95 latency, feasibility assessment per provider

## Global Constraints

- **Pure-Go hard gate**: any activation verdict MUST NOT introduce CGO dependencies
- **Time-box**: 1 day; if real provider activation exceeds it, document blockers and defer
- **Spike branch**: `spike/embeddings-eval`; only decision report + eval-set fixture merge to main
- **Eval-set reuse**: use A2A3 TOML format and runner; don't invent parallel infrastructure
- **Conventional commits**: `docs(spike): ...` for the report
- **English**: all code, tests, report in English

---

## Task 1: Research Real Pure-Go Embedding Providers

**Files:**
- `docs/research/embedding-provider-research.md` (create — findings per candidate)

**Interfaces:**
- Each candidate: language/framework, CGO requirement, operator coverage, local-first, timebox feasibility

**Steps:**

- [ ] **1.1 Research gonnx (pure-Go ONNX inference)**
  ```bash
  # Investigate https://github.com/owulveryck/gonnx
  go get -d github.com/owulveryck/gonnx@latest
  # Check: does it support all-MiniLM-L6-v2 operators (matrix mult, normalization, etc.)?
  # grep -i "operator\|support" $(go env GOPATH)/pkg/mod/github.com/owulveryck/gonnx@*/README.md
  # Document: supported operators, missing operators, example usage
  ```
  **Expected findings**: gonnx may have incomplete operator coverage for transformer models. Document which operators are missing.

- [ ] **1.2 Research ollama sidecar (external process, pure-Go binary)**
  ```bash
  # Investigate https://ollama.ai / https://github.com/ollama/ollama
  # Check: can ollama serve all-MiniLM-L6-v2 embeddings? Does it expose HTTP API?
  # Evaluate tradeoff: subprocess management, startup time, IPC cost
  # Document: startup time, API latency, process lifecycle
  ```
  **Expected findings**: ollama provides HTTP API but requires separate binary install. Tradeoff: operational complexity vs. offload embedding to mature tool.

- [ ] **1.3 Research other candidates**
  ```bash
  # Quick scan for pure-Go alternatives:
  # - huggingface-tinyml? (search: "huggingface go embedding")
  # - local transformers.js via wasm? (out of scope, JS not Go)
  # - sentence-transformers quantized models? (need inference runtime first)
  # Document: ruled out, not applicable, or candidate for deeper investigation
  ```

- [ ] **1.4 Document feasibility summary** in `docs/research/embedding-provider-research.md`:
  ```markdown
  # Pure-Go Embedding Provider Feasibility

  ## Candidates Evaluated

  ### gonnx (Pure-Go ONNX)
  - **Status**: ❌ / ✅ [incomplete operators for transformer models / sufficient coverage]
  - **Timebox fit**: [yes / no — requires operator implementation]
  - **CGO**: No (pure Go)
  - **Key findings**: [which operators missing, what needs implementation]

  ### ollama (Sidecar HTTP API)
  - **Status**: ⚠️ External process
  - **Timebox fit**: [yes — can prototype with existing ollama binary]
  - **CGO**: No (backscroll remains pure-Go; ollama is separate binary)
  - **Tradeoff**: subprocess management, startup ~5-10sec, HTTP overhead ~10ms per request
  - **Key findings**: [ollama serves embeddings over HTTP; pure-Go client needed]

  ### Others
  - [ruled out, why]

  ## Verdict per Candidate

  **Feasible in timebox**: [gonnx | ollama | none]  
  **Recommended for spike measurement**: [if any feasible, pick one]
  **Fallback**: if none feasible, spike measures BM25-only baseline, defer activation, document blockers
  ```

---

## Task 2: Baseline Measurement — BM25-Only (Lexical)

**Files:**
- A2A3 eval-set: `docs/eval/queries.toml` (from A2A3 plan, or create minimal fixture)
- Runner script: `scripts/eval.sh` (from A2A3 plan, or create minimal wrapper)

**Interfaces:**
- Eval record: query string, expected result identifiers (rank/path/snippet)
- Measurement: recall@5 (fraction of queries with expected result at rank ≤5), p95 latency

**Steps:**

- [ ] **2.1 Use or create eval-set fixture** (A2A3 should have created `docs/eval/queries.toml`)
  If A2A3 not completed yet, create minimal fixture with 5-10 real queries:
  ```toml
  [[queries]]
  id = "eval_001"
  query = "pytest auth module tests"
  expected_results = ["pytest", "auth", "test"]  # keywords expected in top-5
  expected_class = "tool"
  ```

- [ ] **2.2 Baseline BM25 measurement**:
  ```bash
  # Run eval-set with lexical-only (no vectors)
  # Command: backscroll search --text "<query>" --lexical-only --json
  # For each query, check if any top-5 result contains expected keywords
  # Compute: recall@5 = (queries with hit at rank ≤5) / total_queries
  
  # Example measurement script (bash):
  #!/bin/bash
  hits=0
  latencies=()
  for query in "pytest auth" "deploy backscroll" "error sqlite locked"; do
    start=$(date +%s%N)
    backscroll search --text "$query" --lexical-only --json --limit 5 > /tmp/result.json
    elapsed=$(($(date +%s%N) - start))
    latencies+=($elapsed)
    # Count if result found (pseudo-logic)
    hits=$((hits + 1))
  done
  recall=$(echo "scale=3; $hits / ${#latencies[@]}" | bc)
  echo "baseline_recall_5=$recall"
  # Compute p95 latency
  ```

- [ ] **2.3 Record baseline metrics** to `/tmp/spike-metrics.txt`:
  ```
  baseline_recall_5=0.XXX
  baseline_p95_latency_ms=XXX
  baseline_binary_size_bytes=XXXXXXX
  ```

---

## Task 3: Evaluate Feasible Real Provider + Measurement (IF Feasible)

**Files:**
- `internal/embedding/spike_provider.go` (create, if real provider is feasible)
- `cmd/backscroll/spike_embedding_test.go` (test harness, inject provider and measure)

**Interfaces:**
- Real `EmbeddingProvider` implementation (gonnx, ollama client, or other)
- Measurement: recall@5 with hybrid, p95 latency, binary size delta

**Steps:**

**CONDITIONAL**: only execute if Task 1 identifies a feasible provider.

- [ ] **3.1 Implement real provider wrapper** (example: ollama client)
  ```go
  // internal/embedding/spike_provider.go (spike-only, not in production)
  package embedding

  import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
  )

  // OllamaEmbeddingProvider calls ollama HTTP API for embeddings
  type OllamaEmbeddingProvider struct {
    endpoint string
    model    string
    client   *http.Client
  }

  func NewOllamaProvider(endpoint, model string) *OllamaEmbeddingProvider {
    return &OllamaEmbeddingProvider{
      endpoint: endpoint,
      model:    model,
      client:   &http.Client{},
    }
  }

  func (o *OllamaEmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
    // POST to ollama API, parse response
    // Return 384-dim embedding or error
    // Implementation example:
    reqBody := map[string]string{"model": o.model, "prompt": text}
    resp, err := o.client.Post(o.endpoint+"/api/embeddings", "application/json", nil)
    if err != nil {
      return nil, fmt.Errorf("ollama embed: %w", err)
    }
    defer resp.Body.Close()
    var result struct {
      Embedding []float32 `json:"embedding"`
    }
    json.NewDecoder(resp.Body).Decode(&result)
    return result.Embedding, nil
  }

  func (o *OllamaEmbeddingProvider) Dimensions() int { return 384 }
  func (o *OllamaEmbeddingProvider) Close() error { return nil }
  ```

- [ ] **3.2 Test harness: inject real provider, measure hybrid vs baseline**
  ```go
  // cmd/backscroll/spike_embedding_test.go (spike-only)
  package main

  import (
    "sort"
    "testing"
    "time"

    "github.com/pablontiv/backscroll/internal/config"
    "github.com/pablontiv/backscroll/internal/embedding"
    "github.com/pablontiv/backscroll/internal/storage"
  )

  // TestHybridWithRealProvider measures hybrid search with real embeddings
  func TestHybridWithRealProvider(t *testing.T) {
    cfg, _ := config.Load()
    db, _ := storage.Open(cfg.DatabasePath)
    defer db.Close()

    // Inject real provider (example: ollama)
    // NOTE: requires ollama server running at localhost:11434
    provider := embedding.NewOllamaProvider("http://localhost:11434", "all-MiniLM-L6-v2")
    db.SetEmbeddingProvider(provider)

    // Load eval-set (from A2A3)
    es, _ := loadEvalSetTOML("docs/eval/queries.toml")

    // Measure hybrid search
    var totalRecall float64
    latencies := make([]int64, 0)

    for _, eq := range es.Queries {
      start := time.Now()
      results, err := db.HybridSearch(eq.Query, models.SearchOptions{
        AllProjects: true,
        Limit:       5,
        LexicalOnly: false,
      })
      elapsed := time.Since(start).Milliseconds()
      latencies = append(latencies, elapsed)

      if err != nil {
        t.Logf("query %s: error %v", eq.ID, err)
        continue
      }

      // Compute recall@5 (fraction of expected results found in top-5)
      var hits int
      for _, sr := range results {
        for _, expected := range eq.ExpectedResults {
          if matchesKeyword(sr.Text, expected) {
            hits++
            break
          }
        }
      }
      recall := float64(hits) / float64(len(eq.ExpectedResults))
      totalRecall += recall
      t.Logf("query %s: recall %.3f, latency %dms", eq.ID, recall, elapsed)
    }

    // Compute aggregate metrics
    avgRecall := totalRecall / float64(len(es.Queries))

    // Compute p95 latency
    sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
    p95Idx := int(float64(len(latencies)) * 0.95)
    if p95Idx >= len(latencies) {
      p95Idx = len(latencies) - 1
    }
    p95Latency := latencies[p95Idx]

    t.Logf("hybrid_recall_5=%.3f", avgRecall)
    t.Logf("hybrid_p95_latency_ms=%d", p95Latency)
  }

  func loadEvalSetTOML(path string) ([]EvalQuery, error) {
    // Load TOML, return slice of EvalQuery
    // Reuse A2A3 evaluation data structure
    return nil, nil // stub
  }

  type EvalQuery struct {
    ID               string
    Query            string
    ExpectedResults  []string
    ExpectedClass    string
  }

  func matchesKeyword(text, keyword string) bool {
    return strings.Contains(text, keyword)
  }
  ```

- [ ] **3.3 Run measurement**:
  ```bash
  # If using ollama:
  ollama pull all-MiniLM-L6-v2
  ollama serve &  # background process
  sleep 5  # wait for startup

  go test -run TestHybridWithRealProvider ./cmd/backscroll -v | tee /tmp/hybrid.txt
  
  # Extract metrics
  grep "hybrid_" /tmp/hybrid.txt >> /tmp/spike-metrics.txt
  
  # Kill ollama
  pkill ollama
  ```

- [ ] **3.4 Record hybrid metrics**:
  ```
  hybrid_recall_5=0.XXX
  hybrid_p95_latency_ms=XXX
  hybrid_provider=[ollama|gonnx|other]
  ```

---

## Task 4: Measure Setup Weight (Binary Size, Provider Startup, Index Build)

**Files:**
- `/tmp/spike-metrics.txt` (append)

**Steps:**

- [ ] **4.1 Binary size delta**:
  ```bash
  # Baseline (already from Task 2)
  baseline_size=$(stat -f%z /tmp/backscroll-baseline 2>/dev/null || stat -c%s /tmp/backscroll-baseline)
  
  # With real provider code (if added)
  go build -o /tmp/backscroll-hybrid ./cmd/backscroll
  hybrid_size=$(stat -f%z /tmp/backscroll-hybrid)
  
  delta=$((hybrid_size - baseline_size))
  echo "binary_size_delta_bytes=$delta" >> /tmp/spike-metrics.txt
  ```

- [ ] **4.2 Provider setup cost** (provider-specific):
  ```bash
  # Example: ollama
  # - Download time for model: ~2-5 minutes (depends on network)
  # - Model size: ~30MB for all-MiniLM-L6-v2
  # - Startup time: measure time from `ollama serve` to first request
  
  time ollama serve > /dev/null 2>&1 &
  pid=$!
  sleep 1
  start=$(date +%s)
  # Ping ollama until responsive
  while ! curl -s http://localhost:11434/api/tags > /dev/null; do sleep 0.1; done
  startup_time=$(($(date +%s) - start))
  kill $pid
  
  echo "provider_startup_time_sec=$startup_time" >> /tmp/spike-metrics.txt
  echo "provider_model_size_bytes=31457280" >> /tmp/spike-metrics.txt  # all-MiniLM-L6-v2 ~30MB
  ```

- [ ] **4.3 Index build time with embeddings**:
  ```bash
  # If T036 is implemented (sync-time vector population):
  # time backscroll sync --project <project>
  # Otherwise, skip (embeddings not yet wired to sync)
  
  echo "index_build_time_sec=TBD" >> /tmp/spike-metrics.txt
  ```

---

## Task 5: Write Decision Report

**Files:**
- `docs/research/2026-07-embeddings-spike.md` (create — final decision with measured metrics)

**Steps:**

- [ ] **5.1 Collect all metrics from `/tmp/spike-metrics.txt`**

- [ ] **5.2 Write report using template**:
  ```markdown
  # Embeddings Spike Decision Report

  Date: 2026-07-02  
  Spike Branch: `spike/embeddings-eval`  
  Decision: [ACTIVATE | DEFER | DELETE]

  ## Executive Summary

  This spike evaluated feasibility of a real, local, pure-Go embedding provider
  for M1 Episodic Recall v1.

  **Verdict**: [ACTIVATE | DEFER | DELETE]  
  **Rationale**: [see Findings section]

  ## Feasibility Assessment

  ### Candidates Researched

  | Candidate | CGO | Local | Feasible in Timebox | Notes |
  |-----------|-----|-------|---------------------|-------|
  | gonnx | No | Yes | [yes/no] | [operator coverage gap / sufficient] |
  | ollama | No* | Yes** | [yes/no] | [sidecar process / subprocess mgmt cost] |
  | [other] | - | - | [yes/no] | [reason] |

  *ollama is separate binary, backscroll remains pure-Go.
  **local first sidecar process.

  ### Chosen Provider (if feasible)

  **Provider**: [gonnx | ollama | none]  
  **Rationale**: [why this one / why none feasible]

  ## Measured Results

  ### BM25-Only Baseline (Lexical)

  | Metric | Value |
  |--------|-------|
  | Recall@5 | 0.XXX |
  | p95 Latency | XXX ms |
  | Binary Size | X.XX MB |

  ### Hybrid (with real provider, if feasible)

  | Metric | Value |
  |--------|-------|
  | Recall@5 | 0.XXX |
  | p95 Latency | XXX ms |
  | Binary Size Delta | +/- X KB |
  | Provider Setup (one-time) | [startup X sec, model X MB] |

  ### Verdict Decision Table

  | Condition | Result |
  |-----------|--------|
  | Real provider feasible? | [YES / NO] |
  | Recall@5 hybrid ≥ baseline? | [YES / NO / N/A] |
  | p95 latency acceptable (≤100ms delta)? | [YES / NO / N/A] |
  | Setup weight acceptable? | [YES / NO / N/A] |

  ## Findings & Analysis

  ### Question 1: Is a real pure-Go embedding provider feasible in 1 day?

  **Answer**: [YES / NO]

  [Detailed findings from Task 1 research and Task 3 implementation attempt]

  **Blockers** (if NO):
  - [gonnx: missing operators for transformers]
  - [ollama: requires external binary installation]
  - [other: reason]

  ### Question 2: If feasible, does hybrid improve recall@5?

  **Answer**: [YES / NO / N/A]

  [Comparison of baseline recall vs. hybrid recall]

  **Delta**: [+0.XXX | -0.XXX | N/A]

  ### Question 3: Is latency acceptable?

  **Answer**: [YES / NO / N/A]

  [Baseline p95: XXX ms → Hybrid p95: XXX ms]

  **Delta**: [+/-XXX ms | N/A]

  ## Verdict Justification

  ### IF VERDICT = ACTIVATE

  ✅ **Conditions met:**
  - Real provider is feasible within pure-Go constraint
  - Recall@5 hybrid ≥ baseline (or improvement justified by other quality gains)
  - p95 latency acceptable (delta ≤ 100ms)
  - Setup weight acceptable (model < 50MB, startup < 30sec)

  🔧 **Next steps (M2):**
  - Wire T036 (sync-time vector population)
  - Deploy real provider (gonnx or ollama sidecar)
  - Grow eval-set to 50 queries covering all content types
  - Tune ranking (recency boost, per-type weights)

  ### IF VERDICT = DEFER

  ⏸️ **Conditions unmet:**
  - [Real provider not feasible in timebox] OR
  - [Recall gain insufficient (< 5% over baseline)] OR
  - [Latency overhead prohibitive (> 100ms)] OR
  - [Setup weight excessive]

  📋 **Reasons:**
  [List specific blockers]

  **When to revisit:**
  - When pure-Go ONNX inference matures (gonnx operator coverage)
  - When eval-set grows and hybrid ranking shows clearer wins
  - When alternative quality improvements (recency boost, per-type weights) are proven cheaper

  ### IF VERDICT = DELETE

  🗑️ **Conditions unmet:**
  - Pure-Go constraint is impossible to meet AND
  - Activation burden (setup, complexity, maintenance) exceeds value

  📋 **Action:**
  - Delete `internal/embedding/`, `internal/hybrid/`, `internal/chunking/` packages
  - Remove `--similarity-threshold`, `--lexical-only` flags from search command
  - Simplify `HybridSearch()` to `Search()` (lexical-only)
  - Delete O09/O10 tasks from roadmap
  - Document: "Vector search deferred indefinitely; rank improvements via lexical + recency + per-type weights only"

  ## Artifacts

  - **Report**: this file (merges to main)
  - **Eval-set fixture**: `docs/eval/queries.toml` (from A2A3, used by regression gate C2)
  - **Research notes**: `docs/research/embedding-provider-research.md` (reference, may stay in spike branch)
  - **Spike branch code**: `spike/embeddings-eval` (reference, not merged except report)

  ## Sign-Off

  **Spike Owner**: [your name]  
  **Date**: 2026-07-02  
  **Reviewed by**: [TBD]
  ```

- [ ] **5.3 Fill all XXX placeholders** with measured values from `/tmp/spike-metrics.txt`

- [ ] **5.4 Ensure verdict is clear** (not wishy-washy; grounded in measured data + feasibility assessment)

---

## Task 6: Commit Report to Main

**Files:**
- `docs/research/2026-07-embeddings-spike.md` (final)
- `docs/eval/queries.toml` (eval-set fixture, from A2A3 or created by spike)

**Steps:**

- [ ] **6.1 Switch to main branch**:
  ```bash
  git checkout main
  git pull origin main
  ```

- [ ] **6.2 Copy report and eval-set from spike branch**:
  ```bash
  # From spike branch (prior step):
  cp /tmp/2026-07-embeddings-spike.md docs/research/2026-07-embeddings-spike.md
  # Eval-set fixture (if created in spike):
  cp docs/eval/queries.toml docs/eval/queries.toml 2>/dev/null || echo "Using A2A3 fixture"
  ```

- [ ] **6.3 Commit with conventional commit**:
  ```bash
  git add docs/research/2026-07-embeddings-spike.md docs/eval/queries.toml
  git commit -m "docs(spike): embeddings evaluation spike and verdict

  - Researched real pure-Go embedding providers (gonnx, ollama, others)
  - Measured BM25-only baseline recall@5 and p95 latency
  - Evaluated hybrid RRF if provider is feasible
  - Verdict: [ACTIVATE | DEFER | DELETE] with rationale
  - Eval-set fixture for M1 C2 regression gate
  
  Spike code: spike/embeddings-eval (reference only, not merged)"
  ```

- [ ] **6.4 Push to main**:
  ```bash
  git push origin main
  ```

- [ ] **6.5 Verify CI passes**:
  ```bash
  gh run list --limit 1
  ```

---

## Ambiguities Resolved

1. **Eval-set structure**: Reuse A2A3 format (TOML) and runner script (bash); don't invent parallel harness.
2. **Real provider measurement**: only hybrid search results from real providers are evidence; mock vectors prove nothing.
3. **Timebox handling**: if real provider not feasible in 1 day, document blockers and verdict defaults to defer/delete with reasons.
4. **Pure-Go constraint**: any CGO dependency (hugot, onnxruntime_go, sqlite-vec) is automatic no-go; sidecar processes (ollama) are evaluable if they keep the backscroll binary pure-Go.
5. **Spike vs. main**: only decision report + eval-set fixture merge to main; experimental code (provider implementations, test harnesses) stays in spike branch.
6. **Provider candidates**: gonnx (pure-Go ONNX, operator coverage gap), ollama (sidecar HTTP API, subprocess cost), others (research Task 1).

---

## Next Steps (Post-Spike)

- **If ACTIVATE** (M2): Implement T036 (sync-time embedding generation), wire real provider, tune ranking weights.
- **If DEFER or DELETE** (M2): Revise O09/O10; focus on lexical ranking improvements (recency boost, per-type weights).
- **C2 regression gate** (ongoing): eval-set fixture runs before each slice push to detect recall regressions.
