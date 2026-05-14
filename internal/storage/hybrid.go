package storage

import (
	"context"
	"strconv"

	"github.com/pablontiv/backscroll/internal/embedding"
	"github.com/pablontiv/backscroll/internal/hybrid"
	"github.com/pablontiv/backscroll/internal/models"
)

// SetEmbeddingProvider attaches an EmbeddingProvider used by HybridSearch.
func (d *Database) SetEmbeddingProvider(p embedding.EmbeddingProvider) {
	d.embeddingProvider = p
}

// HybridSearch performs BM25 search, optionally fused with vector search via RRF.
// Falls back to BM25-only when:
//   - opts.LexicalOnly is true
//   - no embedding provider is set
//   - no vector embeddings exist in the database
//   - the provider fails to embed the query
func (d *Database) HybridSearch(query string, opts models.SearchOptions) ([]SearchResult, error) {
	bm25Results, err := d.Search(query, opts)
	if err != nil {
		return nil, err
	}

	// Skip vector path when explicitly requested or no provider configured
	if opts.LexicalOnly || d.embeddingProvider == nil {
		return bm25Results, nil
	}

	// Check if any vectors are stored
	vectorCount, err := d.GetVectorCount()
	if err != nil || vectorCount == 0 {
		return bm25Results, nil
	}

	// Embed the query
	queryVec, err := d.embeddingProvider.Embed(context.Background(), query)
	if err != nil {
		// Provider unavailable (e.g. ONNX stub) — fall back to BM25
		return bm25Results, nil
	}

	// Vector search: fetch more candidates than limit for RRF fusion
	topK := opts.Limit * 2
	if topK <= 0 {
		topK = 200
	}
	vecResults, err := d.VectorSearch(queryVec, topK)
	if err != nil {
		return bm25Results, nil
	}

	// Apply similarity threshold
	if opts.SimilarityThreshold > 0 {
		filtered := vecResults[:0]
		for _, vr := range vecResults {
			if vr.Similarity >= opts.SimilarityThreshold {
				filtered = append(filtered, vr)
			}
		}
		vecResults = filtered
	}

	if len(vecResults) == 0 {
		return bm25Results, nil
	}

	// Convert to RRF ranking lists
	bm25Ranking := make([]hybrid.RankResult, len(bm25Results))
	for i, r := range bm25Results {
		bm25Ranking[i] = hybrid.RankResult{ID: strconv.Itoa(r.ID), Score: r.Score}
	}
	vecRanking := make([]hybrid.RankResult, len(vecResults))
	for i, vr := range vecResults {
		vecRanking[i] = hybrid.RankResult{ID: strconv.FormatInt(vr.ItemID, 10), Score: vr.Similarity}
	}

	fused := hybrid.ReciprocatRankFusion(60, bm25Ranking, vecRanking)

	// Map BM25 results by ID for fast lookup
	byID := make(map[string]SearchResult, len(bm25Results))
	for _, r := range bm25Results {
		byID[strconv.Itoa(r.ID)] = r
	}

	// Build final result list: items from BM25 re-ranked by RRF score
	// Items only in vector results (not in BM25) are omitted (no snippet available)
	final := make([]SearchResult, 0, len(fused))
	seen := make(map[string]bool)
	for _, f := range fused {
		if seen[f.ID] {
			continue
		}
		seen[f.ID] = true
		if r, ok := byID[f.ID]; ok {
			r.Score = f.Score
			final = append(final, r)
		}
	}

	// Apply limit
	if opts.Limit > 0 && len(final) > opts.Limit {
		final = final[:opts.Limit]
	}

	return final, nil
}

// HasEmbeddingProvider returns true if an embedding provider has been set.
func (d *Database) HasEmbeddingProvider() bool {
	return d.embeddingProvider != nil
}
