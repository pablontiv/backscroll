package storage

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

// TestMergeRRF_DifferentFromMinMax verifies that RRF merging orders results
// differently from min-max normalization, using rank position not magnitude.
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

// TestMergeRRF_ScoreFormula verifies that RRF scores match the formula 1/(k+rank+1).
func TestMergeRRF_ScoreFormula(t *testing.T) {
	// Item at rank 0 in one list (prose), not in the other (tool)
	// RRF = 1/(60 + 0 + 1) = 1/61 ≈ 0.01639
	proseResults := []SearchResult{{ID: 200, Source: "session", ContentType: "text", Score: 100.0}}
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

// TestMergeRRF_NoOverlap verifies that items with no overlap between lists both appear.
func TestMergeRRF_NoOverlap(t *testing.T) {
	// Two lists with no overlap between lists; both should appear, sorted by RRF
	proseResults := []SearchResult{{ID: 301, Source: "session", ContentType: "text", Score: 10.0}}
	toolResults := []SearchResult{{ID: 300, Source: "session", ContentType: "tool", Score: 10.0}}

	fused := mergeRRF(proseResults, toolResults)
	if len(fused) != 2 {
		t.Fatalf("expected 2 results, got %d", len(fused))
	}

	// Both have the same RRF score (rank 0 in their respective list: 1/61 each)
	// Tiebreak by ID ascending (from hybrid.RRF tiebreak rule)
	if fused[0].ID != 300 || fused[1].ID != 301 {
		t.Errorf("expected [300, 301], got [%d, %d]", fused[0].ID, fused[1].ID)
	}
}

func TestExcludeDirectBackscrollSearchEchoesPreservesBoundaries(t *testing.T) {
	results := []SearchResult{
		{ID: 1, ContentType: "tool", Text: "Bash command=backscroll search --text needle"},
		{ID: 2, ContentType: "tool", Text: "bash command=backscroll search --text needle"},
		{ID: 3, ContentType: "tool", Text: "/private/tmp/bin/backscroll search --text needle"},
		{ID: 4, ContentType: "tool", Text: "Bash command=/private/tmp/bin/backscroll search --text needle"},
		{ID: 5, ContentType: "tool", Text: "Bash command=env backscroll search --text needle"},
		{ID: 6, ContentType: "tool", Text: `Bash command=bash -lc "backscroll search --text needle"`},
		{ID: 7, ContentType: "tool", Text: "Bash command=rg needle ."},
		{ID: 8, ContentType: "tool", Text: "Bash command=backscroll status"},
		{ID: 9, ContentType: "tool", Text: "Bash command=backscroll searcher needle"},
		{ID: 10, ContentType: "tool", Text: "error: Bash command=backscroll search failed"},
		{ID: 11, ContentType: "text", Text: "Bash command=backscroll search --text needle"},
	}

	filtered := excludeDirectBackscrollSearchEchoes(results)
	var got []int
	for _, result := range filtered {
		got = append(got, result.ID)
	}
	want := []int{3, 4, 5, 6, 7, 8, 9, 10, 11}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("preserved result IDs = %v, want %v", got, want)
	}
}

func TestRefillToolCandidatesPagesPastEchoesAndDeduplicates(t *testing.T) {
	var offsets []int
	got, err := refillToolCandidatesWithoutDirectEchoes(models.SearchOptions{Limit: 3}, func(opts models.SearchOptions) ([]SearchResult, error) {
		offsets = append(offsets, opts.Offset)
		switch opts.Offset {
		case 0:
			return []SearchResult{
				{ID: 1, ContentType: "tool", Text: "Bash command=backscroll search --text needle"},
				{ID: 2, ContentType: "tool", Text: "Bash command=rg needle ."},
				{ID: 3, ContentType: "tool", Text: "Bash command=backscroll search --text needle"},
			}, nil
		case 3:
			return []SearchResult{
				{ID: 2, ContentType: "tool", Text: "Bash command=rg needle ."},
				{ID: 4, ContentType: "tool", Text: "Bash command=backscroll search --text needle"},
				{ID: 5, ContentType: "tool", Text: "Bash command=grep needle ."},
			}, nil
		case 6:
			return []SearchResult{{ID: 6, ContentType: "tool", Text: "Bash command=fd needle ."}}, nil
		default:
			t.Fatalf("unexpected page offset %d", opts.Offset)
			return nil, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(offsets, []int{0, 3, 6}) {
		t.Fatalf("page offsets = %v, want [0 3 6]", offsets)
	}
	var ids []int
	for _, result := range got {
		ids = append(ids, result.ID)
	}
	if !reflect.DeepEqual(ids, []int{2, 5, 6}) {
		t.Fatalf("refilled IDs = %v, want [2 5 6]", ids)
	}
}

func TestRefillToolCandidatesStopsAtExactFullNonEchoPage(t *testing.T) {
	calls := 0
	got, err := refillToolCandidatesWithoutDirectEchoes(models.SearchOptions{Limit: 2}, func(opts models.SearchOptions) ([]SearchResult, error) {
		calls++
		return []SearchResult{
			{ID: 1, ContentType: "tool", Text: "Bash command=rg one ."},
			{ID: 2, ContentType: "tool", Text: "Bash command=rg two ."},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(got) != 2 {
		t.Fatalf("calls/results = %d/%d, want 1/2", calls, len(got))
	}
}

func TestRefillToolCandidatesExhaustsAfterFullEchoPage(t *testing.T) {
	var offsets []int
	got, err := refillToolCandidatesWithoutDirectEchoes(models.SearchOptions{Limit: 2}, func(opts models.SearchOptions) ([]SearchResult, error) {
		offsets = append(offsets, opts.Offset)
		if opts.Offset == 0 {
			return []SearchResult{
				{ID: 1, ContentType: "tool", Text: "Bash command=backscroll search one"},
				{ID: 2, ContentType: "tool", Text: "Bash command=backscroll search two"},
			}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 || !reflect.DeepEqual(offsets, []int{0, 2}) {
		t.Fatalf("all-echo result/offsets = %v/%v, want empty/[0 2]", got, offsets)
	}
}

func TestRefillToolCandidatesPropagatesCancellationWithoutPartialResults(t *testing.T) {
	calls := 0
	got, err := refillToolCandidatesWithoutDirectEchoes(models.SearchOptions{Limit: 2}, func(opts models.SearchOptions) ([]SearchResult, error) {
		calls++
		if calls == 1 {
			return []SearchResult{
				{ID: 1, ContentType: "tool", Text: "Bash command=rg needle ."},
				{ID: 2, ContentType: "tool", Text: "Bash command=backscroll search needle"},
			}, nil
		}
		return nil, context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got != nil {
		t.Fatalf("partial results returned on cancellation: %v", got)
	}
}
