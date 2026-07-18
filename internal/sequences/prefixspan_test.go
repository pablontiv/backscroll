package sequences

import (
	"testing"
)

func TestMineSingleItem(t *testing.T) {
	seqs := []Sequence{
		{SessionID: "s1", Items: []string{"A", "B"}},
		{SessionID: "s2", Items: []string{"A", "C"}},
		{SessionID: "s3", Items: []string{"B", "C"}},
	}
	patterns := Mine(seqs, 2, 1, 6)
	if len(patterns) != 3 {
		t.Fatalf("want 3 patterns, got %d", len(patterns))
	}
	// Should find A (support 2), B (support 2), C (support 2)
	support := make(map[string]int)
	for _, p := range patterns {
		if len(p.Items) != 1 {
			t.Errorf("expect 1-patterns, got %v", p.Items)
		}
		support[p.Items[0]] = p.Support
	}
	if support["A"] != 2 || support["B"] != 2 || support["C"] != 2 {
		t.Errorf("support mismatch: %v", support)
	}
}

func TestMineSequence(t *testing.T) {
	// Classic case: pattern A→B appears in 3 sessions, A→C in 2 sessions
	seqs := []Sequence{
		{SessionID: "s1", Items: []string{"A", "B"}},
		{SessionID: "s2", Items: []string{"A", "B"}},
		{SessionID: "s3", Items: []string{"A", "C"}},
		{SessionID: "s4", Items: []string{"A", "B", "D"}},
		{SessionID: "s5", Items: []string{"B"}},
	}
	patterns := Mine(seqs, 2, 2, 6) // min support 2, min length 2, max length 6

	// A→B should appear in s1, s2, s4 (support 3)
	// A→C should appear in s3 (support 1, below threshold)
	// A→B→D appears in s4 only (support 1, below threshold)

	if len(patterns) < 1 {
		t.Fatalf("expected at least 1 pattern with support ≥ 2, got %d", len(patterns))
	}

	found := false
	for _, p := range patterns {
		if len(p.Items) == 2 && p.Items[0] == "A" && p.Items[1] == "B" && p.Support == 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected A→B with support 3, patterns: %+v", patterns)
	}
}

func TestMineSorting(t *testing.T) {
	seqs := []Sequence{
		{SessionID: "s1", Items: []string{"X", "Y"}},
		{SessionID: "s2", Items: []string{"X", "Y"}},
		{SessionID: "s3", Items: []string{"X", "Z"}},
	}
	patterns := Mine(seqs, 1, 1, 6)

	// X→Y appears 2 times, X→Z 1 time. X appears 3 times.
	// Sorted: X (3), X→Y (2), X→Z (1)
	if len(patterns) < 3 {
		t.Fatalf("expected 3+ patterns, got %d", len(patterns))
	}

	// First should be X (support 3)
	if patterns[0].Support != 3 || len(patterns[0].Items) != 1 || patterns[0].Items[0] != "X" {
		t.Errorf("first pattern: %+v", patterns[0])
	}
}

func TestMineDeterminism(t *testing.T) {
	seqs := []Sequence{
		{SessionID: "s1", Items: []string{"A", "B", "C"}},
		{SessionID: "s2", Items: []string{"A", "B"}},
		{SessionID: "s3", Items: []string{"B", "C"}},
	}

	r1 := Mine(seqs, 1, 1, 6)
	r2 := Mine(seqs, 1, 1, 6)

	if len(r1) != len(r2) {
		t.Fatalf("different lengths: %d vs %d", len(r1), len(r2))
	}

	for i, p := range r1 {
		if len(p.Items) != len(r2[i].Items) || p.Support != r2[i].Support {
			t.Errorf("pattern %d differs: %+v vs %+v", i, p, r2[i])
		}
		for j, item := range p.Items {
			if item != r2[i].Items[j] {
				t.Errorf("pattern %d item %d differs", i, j)
			}
		}
	}
}

func TestMineMinLengthFilter(t *testing.T) {
	seqs := []Sequence{
		{SessionID: "s1", Items: []string{"A", "B"}},
		{SessionID: "s2", Items: []string{"A", "B"}},
	}

	patterns1 := Mine(seqs, 1, 1, 6) // include 1-patterns
	patterns2 := Mine(seqs, 1, 2, 6) // only 2+ patterns

	if len(patterns1) == 0 || len(patterns2) == 0 {
		t.Fatalf("both should find patterns")
	}

	// patterns1 should include single items (A, B)
	// patterns2 should include A→B but not A or B alone
	for _, p := range patterns2 {
		if len(p.Items) < 2 {
			t.Errorf("minLen=2 should exclude %v", p)
		}
	}
}

func TestMineRepeatedItemWithinSession(t *testing.T) {
	// Pattern A→B appears twice in s1, but should count once
	seqs := []Sequence{
		{SessionID: "s1", Items: []string{"A", "B", "A", "B"}},
		{SessionID: "s2", Items: []string{"A", "B"}},
	}

	patterns := Mine(seqs, 2, 2, 6)

	// A→B should have support 2 (counted once per session)
	found := false
	for _, p := range patterns {
		if len(p.Items) == 2 && p.Items[0] == "A" && p.Items[1] == "B" && p.Support == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("A→B should be counted once per session, patterns: %+v", patterns)
	}
}

func TestMineMaxLengthTermination(t *testing.T) {
	// REGRESSION TEST: prevent combinatorial explosion in repetitive sessions.
	// Three sessions each with READ/EDIT alternating 20 times = 40 items each.
	// Without maxLen 6, the algorithm would mine patterns like
	// READ→EDIT→READ→EDIT→... up to length 40, creating millions of patterns.
	// With maxLen=6, it terminates quickly.

	items := make([]string, 40)
	for i := 0; i < 40; i++ {
		if i%2 == 0 {
			items[i] = "READ"
		} else {
			items[i] = "EDIT"
		}
	}

	seqs := []Sequence{
		{SessionID: "s1", Items: items},
		{SessionID: "s2", Items: items},
		{SessionID: "s3", Items: items},
	}

	// Mine with maxLen=6 (default). Should complete quickly and all patterns
	// have length <= 6.
	patterns := Mine(seqs, 2, 2, 6)

	maxFound := 0
	for _, p := range patterns {
		if len(p.Items) > maxFound {
			maxFound = len(p.Items)
		}
	}

	if maxFound > 6 {
		t.Errorf("found pattern of length %d exceeds maxLen=6", maxFound)
	}

	// Sanity: at least some patterns should be found (READ→EDIT, etc.)
	if len(patterns) == 0 {
		t.Error("expected patterns, got none")
	}
}

func TestMineEdgeCases(t *testing.T) {
	// Empty sequences
	patterns := Mine(nil, 1, 1, 6)
	if len(patterns) != 0 {
		t.Error("empty input should return no patterns")
	}

	// Min support greater than number of sequences
	seqs := []Sequence{
		{SessionID: "s1", Items: []string{"A"}},
		{SessionID: "s2", Items: []string{"A"}},
	}
	patterns = Mine(seqs, 10, 1, 6)
	if len(patterns) != 0 {
		t.Error("min support > num sequences should return no patterns")
	}

	// minLen > maxLen
	patterns = Mine(seqs, 1, 5, 3)
	if len(patterns) != 0 {
		t.Error("minLen > maxLen should return no patterns")
	}
}
