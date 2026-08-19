package sequences

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// TestEquivalenceVsFullDBRecursion pins that projecting incrementally produces exactly
// the patterns the original full-database recursion did. mineReference below is that
// original algorithm, kept verbatim as the specification of correctness: the fix is a
// performance change and any divergence in output is a bug, not an improvement.
func TestEquivalenceVsFullDBRecursion(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	items := []string{"GIT", "TEST_EXEC", "FILE_READ", "FILE_WRITE", "SEARCH", "NAV", "GO_EXEC"}
	for trial := 0; trial < 200; trial++ {
		n := 3 + rng.Intn(12)
		var seqs []Sequence
		for i := 0; i < n; i++ {
			l := 1 + rng.Intn(14)
			var it []string
			for j := 0; j < l; j++ {
				it = append(it, items[rng.Intn(len(items))])
			}
			seqs = append(seqs, Sequence{SessionID: fmt.Sprintf("s%d", i), Items: it})
		}
		ms := 1 + rng.Intn(3)
		got := Mine(seqs, ms, 1, 4)
		want := mineReference(seqs, ms, 1, 4)
		if key(got) != key(want) {
			t.Fatalf("trial %d diverged\n got: %s\nwant: %s", trial, key(got), key(want))
		}
	}
}

func key(ps []Pattern) string {
	var out []string
	for _, p := range ps {
		out = append(out, fmt.Sprintf("%v:%d", p.Items, p.Support))
	}
	sort.Strings(out)
	return fmt.Sprint(out)
}

// mineReference is the pre-fix algorithm: recursion over the FULL database.
func mineReference(sequences []Sequence, minSupport, minLen, maxLen int) []Pattern {
	if minSupport < 1 || minLen < 1 || maxLen < 1 || len(sequences) == 0 ||
		minSupport > len(sequences) || minLen > maxLen {
		return nil
	}
	var patterns []Pattern
	freq := map[string]int{}
	for _, seq := range sequences {
		seen := map[string]bool{}
		for _, it := range seq.Items {
			if !seen[it] {
				freq[it]++
				seen[it] = true
			}
		}
	}
	var one []Pattern
	for it, c := range freq {
		if c >= minSupport {
			one = append(one, Pattern{Items: []string{it}, Support: c})
		}
	}
	if minLen == 1 {
		patterns = append(patterns, one...)
	}
	for _, b := range one {
		if len(b.Items) < maxLen {
			patterns = append(patterns, refExt(sequences, b, minSupport, minLen, maxLen)...)
		}
	}
	return patterns
}

func refExt(sequences []Sequence, base Pattern, minSupport, minLen, maxLen int) []Pattern {
	var projected []Sequence
	for _, seq := range sequences {
		if pr := project(seq, base.Items); len(pr) > 0 {
			projected = append(projected, Sequence{SessionID: seq.SessionID, Items: pr})
		}
	}
	if len(projected) == 0 {
		return nil
	}
	freq := map[string]int{}
	for _, seq := range projected {
		seen := map[string]bool{}
		for _, it := range seq.Items {
			if !seen[it] {
				freq[it]++
				seen[it] = true
			}
		}
	}
	var patterns []Pattern
	for it, c := range freq {
		if c >= minSupport {
			ext := append([]string(nil), base.Items...)
			ext = append(ext, it)
			if len(ext) >= minLen {
				patterns = append(patterns, Pattern{Items: ext, Support: c})
			}
			if len(ext) < maxLen {
				patterns = append(patterns, refExt(sequences, Pattern{Items: ext}, minSupport, minLen, maxLen)...)
			}
		}
	}
	return patterns
}
