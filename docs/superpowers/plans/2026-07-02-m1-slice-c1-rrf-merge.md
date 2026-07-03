# C1: Reciprocal Rank Fusion Merge — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

## Goal

Replace the min-max BM25 score normalization in the unfiltered branch of `Search()` with Reciprocal Rank Fusion (RRF, k=60). This eliminates the cross-index score incomparability problem: RRF is rank-based and immune to scale differences between tokenizers (porter vs trigram). Change is scoped to `internal/storage/search.go` (lines 326–359) and measurable with the Track A eval-set from slice A3 (once available). Filtered queries (single index) are unchanged.

## Architecture

### Current State

Unfiltered queries (`ContentType == ""`) in `Search()` (line 32–53):
1. Query both `tool_fts` (trigram) and `messages_fts` (porter) independently
2. Retrieve full candidate sets (up to 200 rows each, lines 40–47)
3. Merge via `mergeNormalized()` (line 48):
   - Min-max normalize each list's BM25 scores to [0,1] (line 329–359)
   - Concatenate normalized lists
   - Sort descending by normalized score
4. Apply pagination (line 49)

The problem: BM25 scores are incomparable across different FTS5 tokenizers. A trigram match gives a different BM25 than a porter-stemmed match for the same relevance. Min-max normalization tries to bridge this by rescaling, but the result is **approximate** — cross-index ordering depends on relative score distributions, not actual relevance.

### Proposed State

1. Query both tables (unchanged)
2. Merge via `ReciprocatRankFusion()` from `internal/hybrid` (existing, dormant code):
   - Convert each table's results to `hybrid.RankResult` (ID + score placeholder)
   - Call `ReciprocatRankFusion(60, prose_ranking, tool_ranking)` to fuse rankings by position, not score magnitude
   - RRF score = Σ 1/(60 + rank_i + 1) across both tables — rank-based immunity to score incomparability
   - Results already sorted descending by RRF score
3. Apply pagination (unchanged)

### Why RRF Works Here

- **Rank-based, not score-based**: RRF cares only about *position* in each ranked list. The trigram scorer and porter scorer can disagree wildly on magnitude; RRF just sees "rank 0 in tool_fts" and "rank 5 in messages_fts" and fuses them via a principled formula.
- **Standard constant k=60**: Empirically chosen across retrieval literature; already used in `HybridSearch` (line 82 of `internal/storage/hybrid.go`), so consistency.
- **Revives dormant code**: `internal/hybrid` was designed for O10 (vector fusion). C1 reuses the core RRF logic for the immediate BM25-only case, proving its value before embeddings spike (C3).
- **Local change**: unfiltered branch only; filtered queries continue as-is. No schema, no CLI changes.

## Tech Stack

- **Language**: Go (stdlib + existing internal packages)
- **Imports**: `internal/hybrid` (already imported in `hybrid.go`; add to `search.go`)
- **Testing**: stdlib `testing` + fixtures for multi-index ranking validation
- **Build**: `just check` (gofmt, go vet), `just test`, coverage ≥85%
- **Delivery**: conventional commit, direct to `main`, auto-release via CI

## Global Constraints

- Pure Go, no CGO. (Satisfied: `internal/hybrid` is stdlib only.)
- Tests must pass: `just test ./...`
- Coverage ≥85% per-package enforced by pre-push hook (`pkcov`).
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/). Type: `perf` (performance/ranking improvement).
- Direct commits to `main`; push after completion triggers automated CI release.
- No breaking CLI changes. Flag signature, output format, and JSON schema unchanged.
- CLAUDE.md updated to reflect the new ranking strategy.

## Task List

### Task 1: Unit Test — RRF Merging on Dual Indexes

**Files**:
- `internal/storage/search_test.go` (new test file; add tests alongside existing search tests)
- Test corpus: manually constructed `SearchResult` slices simulating tool_fts and messages_fts results
- **Imports needed for test file**: `"fmt"`, `"math"`, `"testing"`, `"time"` (stdlib); `"github.com/pablontiv/backscroll/internal/hybrid"`

**Interfaces**:
- `mergeRRF(proseResults, toolResults []SearchResult) []SearchResult` — new function to implement (to replace `mergeNormalized`)
- Reuse `hybrid.ReciprocatRankFusion(60, rankings ...[]hybrid.RankResult)`

**Strict TDD Steps**:

- [ ] **Write failing test**: `TestMergeRRF_DifferentFromMinMax`
  ```go
  func TestMergeRRF_DifferentFromMinMax(t *testing.T) {
  	// Construct a corpus where min-max and RRF order items differently.
  	// Key: ID=100 ranks high in tool_fts but low in messages_fts; ID=103 ranks mid in both.
  	
  	// Simulate tool_fts: ID=100 (rank 0, score 100), ID=101 (rank 1, score 90), ID=102 (rank 2, score 80)
  	toolResults := []SearchResult{
  		{ID: 100, Source: "session", ContentType: "tool", Score: 100.0},
  		{ID: 101, Source: "session", ContentType: "tool", Score: 90.0},
  		{ID: 102, Source: "session", ContentType: "tool", Score: 80.0},
  	}
  	
  	// Simulate messages_fts: ID=101 (rank 0, score 100), ID=102 (rank 1, score 90), ID=100 (rank 2, score 10)
  	// Note: ID=100's score is intentionally low here to show min-max vs RRF difference
  	proseResults := []SearchResult{
  		{ID: 101, Source: "session", ContentType: "text", Score: 100.0},
  		{ID: 102, Source: "session", ContentType: "text", Score: 90.0},
  		{ID: 100, Source: "session", ContentType: "text", Score: 10.0},
  	}

  	fused := mergeRRF(proseResults, toolResults)

  	// Expect 3 items
  	if len(fused) != 3 {
  		t.Fatalf("expected 3 results, got %d", len(fused))
  	}

  	// MIN-MAX WOULD ORDER: 101 > 102 > 100
  	// (101 scores high in both; 102 scores high in both; 100 is penalized by low score in messages_fts)
  	//
  	// RRF ORDERS: 101 > 100 > 102
  	// - ID=101: 1/(60+1+1) + 1/(60+0+1) ≈ 0.01613 + 0.01639 = 0.03252 (rank 1 in tool, rank 0 in messages)
  	// - ID=100: 1/(60+0+1) + 1/(60+2+1) ≈ 0.01639 + 0.01587 = 0.03226 (rank 0 in tool, rank 2 in messages)
  	// - ID=102: 1/(60+2+1) + 1/(60+1+1) ≈ 0.01587 + 0.01613 = 0.03200 (rank 2 in tool, rank 1 in messages)
  	// RRF boosts ID=100 because it ranks 0 in tool_fts, despite low score in messages_fts.
  	// Min-max penalizes ID=100 because its normalized score in messages_fts is [0, 1] ≈ 0 (actual 10 → (10-10)/(100-10) = 0).

  	// Verify RRF order: 101 > 100 > 102
  	if fused[0].ID != 101 {
  		t.Errorf("expected ID=101 first (high in both lists), got ID=%d", fused[0].ID)
  	}
  	if fused[1].ID != 100 {
  		t.Errorf("expected ID=100 second (rank 0 in tool despite low score in messages), got ID=%d", fused[1].ID)
  	}
  	if fused[2].ID != 102 {
  		t.Errorf("expected ID=102 third (ranks mid in both), got ID=%d", fused[2].ID)
  	}

  	// Verify RRF scores are rank-based, not min-max normalized
  	for i, r := range fused {
  		if r.Score <= 0 {
  			t.Errorf("result[%d] has invalid RRF score %f", i, r.Score)
  		}
  	}
  }
  ```

- [ ] **Write test for score stability**: `TestMergeRRF_ScoreFormula`
  ```go
  func TestMergeRRF_ScoreFormula(t *testing.T) {
  	// Item at rank 0 in one list (prose), not in the other (tool)
  	// RRF = 1/(60 + 0 + 1) = 1/61 ≈ 0.01639
  	proseResults := []SearchResult{{ID: 200, Score: 100}}
  	toolResults := []SearchResult{}

  	fused := mergeRRF(proseResults, toolResults)
  	if len(fused) != 1 {
  		t.Fatalf("expected 1 result")
  	}

  	expected := 1.0 / 61.0 // 1/(k + rank + 1), rank=0, k=60
  	if math.Abs(fused[0].Score-expected) > 1e-9 {
  		t.Errorf("expected RRF score %.10f, got %.10f", expected, fused[0].Score)
  	}
  }
  ```

- [ ] **Write test for no overlap**: `TestMergeRRF_NoOverlap`
  ```go
  func TestMergeRRF_NoOverlap(t *testing.T) {
  	// Two lists with no common IDs; both should appear, sorted by RRF
  	proseResults := []SearchResult{{ID: 301, Score: 10}}
  	toolResults := []SearchResult{{ID: 300, Score: 10}}

  	fused := mergeRRF(proseResults, toolResults)
  	if len(fused) != 2 {
  		t.Fatalf("expected 2 results")
  	}

  	// Both have the same RRF score (rank 0 in their respective list)
  	// Tiebreak by ID ascending (from hybrid.RRF tiebreak rule)
  	if fused[0].ID != 300 || fused[1].ID != 301 {
  		t.Errorf("expected [300, 301], got [%d, %d]", fused[0].ID, fused[1].ID)
  	}
  }
  ```

- [ ] **Run test suite to verify failures**: `go test -v ./internal/storage/... -run TestMergeRRF`
  - Expected: all three tests fail (function doesn't exist yet)

- [ ] **Implement `mergeRRF()` in `internal/storage/search.go`**:
  ```go
  // mergeRRF uses Reciprocal Rank Fusion to merge two ranked lists by position,
  // immune to score-scale differences between tokenizers (trigram vs porter).
  // k=60 is the standard RRF constant.
  func mergeRRF(proseResults, toolResults []SearchResult) []SearchResult {
  	// Convert to hybrid.RankResult for RRF fusion
  	toolRanking := make([]hybrid.RankResult, len(toolResults))
  	for i, r := range toolResults {
  		toolRanking[i] = hybrid.RankResult{
  			ID:    fmt.Sprintf("%d", r.ID),
  			Score: r.Score, // score value ignored by RRF; rank position is used
  		}
  	}

  	proseRanking := make([]hybrid.RankResult, len(proseResults))
  	for i, r := range proseResults {
  		proseRanking[i] = hybrid.RankResult{
  			ID:    fmt.Sprintf("%d", r.ID),
  			Score: r.Score,
  		}
  	}

  	// Fuse rankings via RRF
  	fused := hybrid.ReciprocatRankFusion(60, toolRanking, proseRanking)

  	// Map fused results back to SearchResult with RRF score
  	// Create ID→SearchResult lookup from original results
  	byID := make(map[string]*SearchResult)
  	for i := range toolResults {
  		key := fmt.Sprintf("%d", toolResults[i].ID)
  		byID[key] = &toolResults[i]
  	}
  	for i := range proseResults {
  		key := fmt.Sprintf("%d", proseResults[i].ID)
  		// If already in byID (overlap), keep the existing entry's pointer
  		if _, exists := byID[key]; !exists {
  			byID[key] = &proseResults[i]
  		}
  	}

  	// Build final list in RRF order
  	final := make([]SearchResult, 0, len(fused))
  	for _, f := range fused {
  		if r, ok := byID[f.ID]; ok {
  			result := *r
  			result.Score = f.Score // Replace with RRF score
  			final = append(final, result)
  		}
  	}

  	return final
  }
  ```

- [ ] **Run tests to verify passes**: `go test -v ./internal/storage/... -run TestMergeRRF`
  - Expected: all three tests pass

- [ ] **Verify coverage**: `go test -cover ./internal/storage`
  - Target: search.go coverage remains ≥85%

---

### Task 2: Integration Test — Full Unfiltered Search with RRF

**Files**:
- `internal/storage/storage_test.go` (extend existing integration tests)

**Strict TDD Steps**:

- [ ] **Write failing test**: `TestSearch_UnfilteredMergesToolAndProseWithRRF`
  ```go
  func TestSearch_UnfilteredMergesToolAndProseWithRRF(t *testing.T) {
  	db, cleanup := newTestDB(t)
  	defer cleanup()

  	// Insert a mixed corpus:
  	// - Item A: tool content, high in tool_fts
  	// - Item B: prose content, high in messages_fts, also has tool mention
  	// - Item C: tool only, lower rank in tool_fts
  	// - Item D: prose only, lower rank in messages_fts

  	now := time.Now()
  	items := []struct {
  		id          int
  		source      string
  		contentType string
  		text        string
  		project     string
  	}{
  		{1, "session", "tool", "error: connection timeout failed", "proj1"},
  		{2, "session", "text", "we debugged the connection timeout issue for hours", "proj1"},
  		{3, "session", "tool", "retry mechanism implemented success", "proj1"},
  		{4, "session", "code", "fixed a typo in the config", "proj1"},
  	}

  	for _, item := range items {
  		err := db.db.Exec(`
  			INSERT INTO search_items (source, source_path, ordinal, role, text, snippet, timestamp, uuid, project, content_type)
  			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  		`, item.source, "path/"+item.source, 1, "assistant", item.text, item.text, now.Format(time.RFC3339), 
  		   fmt.Sprintf("uuid-%d", item.id), item.project, item.contentType).Err
  		if err != nil {
  			t.Fatalf("insert item %d: %v", item.id, err)
  		}
  	}

  	// Search for "timeout" — appears in tool content (item 1) and prose (item 2)
  	opts := models.SearchOptions{
  		Project: "proj1",
  		Limit:   100,
  	}
  	results, err := db.Search("timeout", opts)
  	if err != nil {
  		t.Fatalf("search: %v", err)
  	}

  	// Expect results in RRF order:
  	// - Both item 1 and 2 should be high (both matched "timeout")
  	// - RRF should weight them based on rank in their respective indexes
  	if len(results) < 2 {
  		t.Fatalf("expected at least 2 results with 'timeout', got %d", len(results))
  	}

  	// Verify: first two results contain items 1 and 2 (in RRF order)
  	topIDs := map[int]bool{results[0].ID: true, results[1].ID: true}
  	if !topIDs[1] || !topIDs[2] {
  		t.Errorf("expected items 1 and 2 in top 2, got IDs: %v", 
  			[]int{results[0].ID, results[1].ID})
  	}

  	// Verify scores are RRF (not min-max normalized [0, 1])
  	for _, r := range results {
  		if r.Score <= 0 {
  			t.Errorf("result ID %d has invalid RRF score %f", r.ID, r.Score)
  		}
  	}
  }
  ```

- [ ] **Run test to verify failure**: `go test -v ./internal/storage/... -run TestSearch_UnfilteredMergesToolAndProseWithRRF`
  - Expected: fail (current code uses min-max)

- [ ] **Update `Search()` to use `mergeRRF()`** in `internal/storage/search.go` (lines 38–49):
  ```go
  case "":
  	// Unfiltered search: query both tables and merge via RRF
  	prose, err := d.searchTable("messages_fts", query, withoutPaging(opts))
  	if err != nil {
  		return nil, err
  	}
  	tool, err := d.searchTable("tool_fts", query, withoutPaging(opts))
  	if err != nil {
  		return nil, err
  	}
  	merged := mergeRRF(prose, tool)  // Changed from mergeNormalized
  	return paginate(merged, opts.Limit, opts.Offset), nil
  ```

- [ ] **Remove obsolete functions**: delete `mergeNormalized()` (lines 329–335) and `normalize()` (lines 337–359)
  - Verify no other callers exist: `rg "mergeNormalized\|normalize\(" internal/ --type go`

- [ ] **Run integration test to verify pass**: `go test -v ./internal/storage/... -run TestSearch_UnfilteredMergesToolAndProseWithRRF`
  - Expected: pass

- [ ] **Run full test suite**: `just test`
  - Expected: all tests pass, coverage ≥85%

---

### Task 3: Add Import for hybrid Package

**Files**:
- `internal/storage/search.go` (add import)

**Steps**:

- [ ] **Add import**:
  ```go
  import (
  	"fmt"
  	"sort"
  	"strings"
  	"time"

  	"github.com/pablontiv/backscroll/internal/hybrid"  // NEW
  	"github.com/pablontiv/backscroll/internal/models"
  )
  ```

- [ ] **Verify no circular dependencies**: `go build ./...`

---

### Task 4: Verify Rank Stability with eval-set (Once Available)

**Files**:
- `scripts/eval.sh` (created in slice A3; if not yet available, document the step as "pending A3")
- `docs/eval/queries.json` (created in slice A3; corpus of ~20 hand-annotated queries)

**Conditional Step** (only runs after slice A3 is merged):

- [ ] **Baseline before change**: Run eval-set against `mergeNormalized` version (establish baseline recall@5)
  ```bash
  BACKSCROLL_DB=/tmp/baseline.db just build && \
  backscroll rebuild --config ./inputs/*.inputs.toml && \
  bash scripts/eval.sh recall@5
  ```
  - Expected: e.g., "recall@5: 0.92"

- [ ] **Run after RRF merge**: Run eval-set against `mergeRRF` version
  ```bash
  BACKSCROLL_DB=/tmp/rrf.db just build && \
  backscroll rebuild --config ./inputs/*.inputs.toml && \
  bash scripts/eval.sh recall@5
  ```
  - Expected: equal or improved (RRF at worst maintains, at best improves due to rank-based fusion)

- [ ] **Document before/after**: Create a short report in the commit message or as a `perf` commit comment:
  - Baseline recall@5: `X%`
  - RRF recall@5: `Y%`
  - Conclusion: "RRF maintains corpus retrieval quality (rank-based fusion immune to tokenizer scale differences)"

---

### Task 5: Update CLAUDE.md to Reflect RRF Strategy

**Files**:
- `/Users/Shared/harness/backscroll/CLAUDE.md` (line 98, "Split FTS by retrieval semantics" section)

**Steps**:

- [ ] **Find and replace the min-max wording**:
  Current text (around line 198):
  ```
  ... use separate FTS5 indexes: tool_fts (tokenizer `trigram`, substring/exact match for paths/commands/errors); text+code live in `messages_fts` (`porter unicode61`). `content_type`-branched triggers route each row. `--content-type tool` queries `tool_fts`; prose queries `messages_fts`; an unfiltered query merges both by per-table min-max-normalized BM25 (cross-index ordering is approximate).
  ```

  Replace with:
  ```
  ... use separate FTS5 indexes: tool_fts (tokenizer `trigram`, substring/exact match for paths/commands/errors); text+code live in `messages_fts` (`porter unicode61`). `content_type`-branched triggers route each row. `--content-type tool` queries `tool_fts`; prose queries `messages_fts`; an unfiltered query merges both via Reciprocal Rank Fusion (RRF, k=60), which fuses by rank position, not score magnitude, and is immune to incomparable cross-tokenizer BM25 scales.
  ```

- [ ] **Verify line numbers and context match**: `rg "min-max-normalized BM25" /Users/Shared/harness/backscroll/CLAUDE.md`

- [ ] **No other references to update**: `rg "min-max\|normalize.*score" /Users/Shared/harness/backscroll/CLAUDE.md`

---

### Task 6: Commit and Verify Final State

**Files**:
- All modified: `internal/storage/search.go`, `internal/storage/search_test.go` (extended), `CLAUDE.md`
- Deleted code: `normalize()`, `mergeNormalized()` functions

**Steps**:

- [ ] **Run pre-push gate**: `just check`
  - Expected: gofmt and go vet pass

- [ ] **Run test suite**: `just test`
  - Expected: all tests pass, including new RRF tests

- [ ] **Run coverage check**: `just coverage-check`
  - Expected: all packages ≥85%, internal/storage in particular

- [ ] **Stage changes**:
  ```bash
  git add internal/storage/search.go internal/storage/search_test.go CLAUDE.md
  ```

- [ ] **Commit with conventional message**:
  ```bash
  git commit -m "perf(storage): Replace min-max normalization with RRF for unfiltered dual-index merge

  Unfiltered searches (ContentType == '') now merge tool_fts and messages_fts via
  Reciprocal Rank Fusion (RRF, k=60) instead of min-max score normalization. RRF
  is rank-based and immune to BM25 scale differences between the trigram and porter
  tokenizers, eliminating approximate ordering.

  - Implement mergeRRF() using existing internal/hybrid.ReciprocatRankFusion()
  - Remove obsolete normalize() and mergeNormalized() functions
  - Add unit tests for RRF merging across tool and prose result sets
  - Update CLAUDE.md to reflect rank-based fusion strategy
  - Coverage ≥85% maintained per package; all tests pass

  Filtered queries (--content-type tool|text|code|reasoning) unchanged.
  "
  ```

- [ ] **Verify commit**: `git log -1 --oneline`
  - Expected: "perf(storage): Replace min-max normalization with RRF..."

- [ ] **Push to main** (automatic CI release on push):
  ```bash
  git push origin main
  ```

---

## Dormant Code Decision

The dormant O10 RRF code in `internal/hybrid` is **revived and reused** in this slice for BM25-only dual-index fusion. The same `ReciprocatRankFusion()` function will later be used in C3's embeddings spike for vector+BM25 hybrid fusion. Do NOT delete any code from `internal/hybrid/`; it is the nucleus for future hybrid retrieval work.

If C3 (embeddings spike) decides **not** to activate embeddings, the O09/O10 dormant code (`internal/embedding/`, vector tables, `HybridSearch()` method) will be evaluated for deletion in a separate C3 decision artifact. That is out of scope for C1.

---

## Success Criteria

✓ All new RRF unit tests pass  
✓ Integration test verifies unfiltered search uses rank-based fusion  
✓ `just check` passes (formatting, linting)  
✓ `just test` passes (100% of test suite)  
✓ Coverage ≥85% per package  
✓ CLAUDE.md updated with RRF terminology  
✓ Commit follows Conventional Commits (type: `perf`)  
✓ eval-set recall@5 measured once A3 slice is available (not a gate; advisory)  

---

## Ambiguities Resolved

1. **Score vs Rank**: RRF uses *rank position*, not BM25 magnitude. Input BM25 scores are converted to positions (0-indexed rank in the sorted list) inside `mergeRRF()`. ✓

2. **Cross-tokenizer incomparability**: Min-max assumed two tokenizers produce comparable score ranges within the same dataset. They don't (trigram substring matches give different magnitudes than porter lemma matches). RRF ignores magnitude entirely and uses position only. ✓

3. **Dormant code**: O10's `ReciprocatRankFusion()` exists and is production-ready; reuse it, don't reimplement. Its test suite validates the formula. ✓

4. **Filtered vs unfiltered**: Only the unfiltered branch (empty ContentType) changes. Filtered queries (`--content-type tool`, `--content-type text`, etc.) continue to query a single FTS table and return its BM25 scores unchanged. ✓

5. **k=60 constant**: Standard RRF parameter; matches the value used in `HybridSearch()` for consistency. ✓

6. **Evaluation**: Recall@5 measurement requires the eval-set from A3. If A3 is delayed, C1 ships with RRF code verified by unit and integration tests; eval-set measurement is an advisory follow-up once A3 is available. ✓

