use std::collections::HashMap;

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct RankedItem {
    pub id: i64,
    pub score: f64,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct FusedItem {
    pub id: i64,
    pub rrf_score: f64,
}

/// Reciprocal Rank Fusion: score(d) = Σ 1/(k + rank_i(d) + 1)
#[allow(dead_code)]
pub(crate) fn reciprocal_rank_fusion(rankings: &[Vec<RankedItem>], k: usize) -> Vec<FusedItem> {
    let mut scores: HashMap<i64, f64> = HashMap::new();
    for ranking in rankings {
        for (rank, item) in ranking.iter().enumerate() {
            *scores.entry(item.id).or_insert(0.0) += 1.0 / (k as f64 + rank as f64 + 1.0);
        }
    }
    let mut fused: Vec<FusedItem> = scores
        .into_iter()
        .map(|(id, rrf_score)| FusedItem { id, rrf_score })
        .collect();
    fused.sort_by(|a, b| {
        b.rrf_score
            .partial_cmp(&a.rrf_score)
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    fused
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_ranking(ids: &[i64]) -> Vec<RankedItem> {
        ids.iter()
            .enumerate()
            .map(|(rank, &id)| RankedItem {
                id,
                score: 1.0 / (rank as f64 + 1.0),
            })
            .collect()
    }

    #[test]
    fn test_single_ranking() {
        let ranking = make_ranking(&[10, 20, 30]);
        let fused = reciprocal_rank_fusion(&[ranking], 60);
        assert_eq!(fused.len(), 3);
        // Order should be preserved: rank 0 > rank 1 > rank 2
        assert_eq!(fused[0].id, 10);
        assert_eq!(fused[1].id, 20);
        assert_eq!(fused[2].id, 30);
        // Scores strictly decreasing
        assert!(fused[0].rrf_score > fused[1].rrf_score);
        assert!(fused[1].rrf_score > fused[2].rrf_score);
    }

    #[test]
    fn test_two_rankings_boost_overlap() {
        // Item 99 appears in both rankings at rank 0 — should have highest score
        let ranking_a = make_ranking(&[99, 1, 2]);
        let ranking_b = make_ranking(&[99, 3, 4]);
        let fused = reciprocal_rank_fusion(&[ranking_a, ranking_b], 60);
        assert_eq!(fused[0].id, 99);
        // Score of 99 = 2 * 1/(60+0+1) — higher than any single-list item
        let item_1 = fused.iter().find(|f| f.id == 1).unwrap();
        assert!(fused[0].rrf_score > item_1.rrf_score);
    }

    #[test]
    fn test_empty_rankings() {
        let fused = reciprocal_rank_fusion(&[], 60);
        assert!(fused.is_empty());

        let fused2 = reciprocal_rank_fusion(&[vec![]], 60);
        assert!(fused2.is_empty());
    }

    #[test]
    fn test_k_parameter_affects_smoothing() {
        // With smaller k, rank differences matter more (wider spread)
        let ranking = make_ranking(&[1, 2]);
        let fused_k1 = reciprocal_rank_fusion(std::slice::from_ref(&ranking), 1);
        let fused_k60 = reciprocal_rank_fusion(std::slice::from_ref(&ranking), 60);

        let spread_k1 = fused_k1[0].rrf_score - fused_k1[1].rrf_score;
        let spread_k60 = fused_k60[0].rrf_score - fused_k60[1].rrf_score;

        // k=1 gives: 1/(1+0+1)=0.5 vs 1/(1+1+1)=0.333, spread=0.167
        // k=60 gives: 1/(60+0+1)=0.0164 vs 1/(60+1+1)=0.0161, spread=~0.0003
        assert!(spread_k1 > spread_k60);
    }
}
