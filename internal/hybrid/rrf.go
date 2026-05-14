// Package hybrid implements Reciprocal Rank Fusion for combining ranked result lists.
package hybrid

import "sort"

// RankResult holds an item ID and its score from a single ranker.
type RankResult struct {
	ID    string
	Score float64
}

// ReciprocatRankFusion combines multiple rankings using RRF (Cormack et al. 2009).
// score(d) = Σ 1/(k + rank_i(d) + 1) across all lists containing d.
// k=60 is the standard constant. Results are returned sorted descending by RRF score.
func ReciprocatRankFusion(k int, rankings ...[]RankResult) []RankResult {
	scores := make(map[string]float64)
	for _, ranking := range rankings {
		for rank, item := range ranking {
			scores[item.ID] += 1.0 / (float64(k) + float64(rank) + 1.0)
		}
	}
	fused := make([]RankResult, 0, len(scores))
	for id, score := range scores {
		fused = append(fused, RankResult{ID: id, Score: score})
	}
	sort.Slice(fused, func(i, j int) bool {
		if fused[i].Score != fused[j].Score {
			return fused[i].Score > fused[j].Score
		}
		return fused[i].ID < fused[j].ID
	})
	return fused
}
