package hybrid

import (
	"math"
	"testing"
)

func TestRRF_SingleRanking(t *testing.T) {
	r := []RankResult{{ID: "A", Score: 1}, {ID: "B", Score: 0.5}, {ID: "C", Score: 0.25}}
	fused := ReciprocatRankFusion(60, r)
	if len(fused) != 3 {
		t.Fatalf("expected 3 results, got %d", len(fused))
	}
	// Order must be preserved: A > B > C (rank 0 > rank 1 > rank 2)
	if fused[0].ID != "A" || fused[1].ID != "B" || fused[2].ID != "C" {
		t.Errorf("wrong order: %v", fused)
	}
}

func TestRRF_TwoRankings_BoostOverlap(t *testing.T) {
	// A is top in list1, B is top in list2; B also appears in list1 rank2
	list1 := []RankResult{{ID: "A", Score: 1}, {ID: "B", Score: 0.5}, {ID: "C", Score: 0.25}}
	list2 := []RankResult{{ID: "B", Score: 1}, {ID: "A", Score: 0.5}, {ID: "D", Score: 0.25}}
	fused := ReciprocatRankFusion(60, list1, list2)
	// Both A and B appear in both lists — B is #1 in list2, A is #1 in list1
	// They should both be in top 2; D only in list2 should be lower than A and B
	if len(fused) != 4 {
		t.Fatalf("expected 4 results, got %d", len(fused))
	}
	top2 := map[string]bool{fused[0].ID: true, fused[1].ID: true}
	if !top2["A"] || !top2["B"] {
		t.Errorf("A and B should be top 2, got %v", fused)
	}
}

func TestRRF_EmptyInput(t *testing.T) {
	fused := ReciprocatRankFusion(60)
	if len(fused) != 0 {
		t.Errorf("expected empty, got %v", fused)
	}
}

func TestRRF_EmptyList(t *testing.T) {
	fused := ReciprocatRankFusion(60, []RankResult{})
	if len(fused) != 0 {
		t.Errorf("expected empty, got %v", fused)
	}
}

func TestRRF_ScoreFormula(t *testing.T) {
	// Single item at rank 0 with k=60: score = 1/(60+0+1) = 1/61
	r := []RankResult{{ID: "X", Score: 1}}
	fused := ReciprocatRankFusion(60, r)
	expected := 1.0 / 61.0
	if math.Abs(fused[0].Score-expected) > 1e-9 {
		t.Errorf("expected score %.10f, got %.10f", expected, fused[0].Score)
	}
}

func TestRRF_DocumentInOnlyOneList(t *testing.T) {
	list1 := []RankResult{{ID: "A", Score: 1}}
	list2 := []RankResult{{ID: "B", Score: 1}}
	fused := ReciprocatRankFusion(60, list1, list2)
	// Both get the same score (both rank 0 in their list)
	if len(fused) != 2 {
		t.Fatalf("expected 2, got %d", len(fused))
	}
	if math.Abs(fused[0].Score-fused[1].Score) > 1e-9 {
		t.Errorf("both single-list items should have equal score")
	}
}
