package sequences

import (
	"sort"
)

// Mine discovers frequent subsequences using PrefixSpan (simplified, correct version).
// Input: per-session item sequences, min support threshold, min pattern length, max pattern length.
// maxLen (default 6) is MANDATORY to prevent combinatorial explosion in repetitive sessions.
// Output: patterns sorted by support (DESC) then lexicographic order.
func Mine(sequences []Sequence, minSupport, minLen, maxLen int) []Pattern {
	if minSupport < 1 || minLen < 1 || maxLen < 1 {
		return nil
	}
	if len(sequences) == 0 {
		return nil
	}

	// Degenerate case: minSupport > len(sequences) means no pattern can be frequent
	if minSupport > len(sequences) {
		return nil
	}

	if minLen > maxLen {
		return nil
	}

	var patterns []Pattern

	// Single-item frequency pass
	freq := make(map[string]int)
	for _, seq := range sequences {
		seen := make(map[string]bool)
		for _, item := range seq.Items {
			if !seen[item] {
				freq[item]++
				seen[item] = true
			}
		}
	}

	// Collect frequent 1-patterns
	var onePatterns []Pattern
	for item, count := range freq {
		if count >= minSupport {
			onePatterns = append(onePatterns, Pattern{
				Items:   []string{item},
				Support: count,
			})
		}
	}

	// Add 1-patterns if they meet minLen
	if minLen == 1 {
		patterns = append(patterns, onePatterns...)
	}

	// Recursively extend each 1-pattern (stop if maxLen reached)
	for _, base := range onePatterns {
		if len(base.Items) < maxLen {
			extended := mineExtensions(sequences, base, minSupport, minLen, maxLen)
			patterns = append(patterns, extended...)
		}
	}

	// Sort: support DESC, then lexicographic
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].Support != patterns[j].Support {
			return patterns[i].Support > patterns[j].Support
		}
		return lexicographic(patterns[i].Items) < lexicographic(patterns[j].Items)
	})

	return patterns
}

// mineExtensions finds longer patterns by extending a base pattern.
// Stops recursing when len(extended) >= maxLen to prevent combinatorial explosion.
func mineExtensions(sequences []Sequence, base Pattern, minSupport, minLen, maxLen int) []Pattern {
	// Compute projected database: sessions containing base, positioned after base's last item
	var projected []Sequence
	for _, seq := range sequences {
		if proj := project(seq, base.Items); len(proj) > 0 {
			projected = append(projected, Sequence{SessionID: seq.SessionID, Items: proj})
		}
	}

	if len(projected) == 0 {
		return nil
	}

	// Recursive case: mine extensions from projected database
	// Step 1: Find frequent single items in the projected database
	freq := make(map[string]int)
	for _, seq := range projected {
		seen := make(map[string]bool)
		for _, item := range seq.Items {
			if !seen[item] {
				freq[item]++
				seen[item] = true
			}
		}
	}

	var patterns []Pattern
	for item, count := range freq {
		if count >= minSupport {
			extended := append([]string(nil), base.Items...)
			extended = append(extended, item)

			// Yield this pattern if it meets minLen
			if len(extended) >= minLen {
				patterns = append(patterns, Pattern{
					Items:   extended,
					Support: count,
				})
			}

			// Recursively extend further (stop if maxLen reached)
			if len(extended) < maxLen {
				subExtended := mineExtensions(sequences, Pattern{Items: extended}, minSupport, minLen, maxLen)
				patterns = append(patterns, subExtended...)
			}
		}
	}

	return patterns
}

// project extracts the suffix of a sequence after the first completion of the base pattern.
// PrefixSpan uses earliest-completion (greedy left-to-right) semantics: find the pattern
// via first-match, then take the suffix after that position. This maximizes the projected
// database size, ensuring no frequent extension is missed by an early completion choice.
// Used to build the projected database for PrefixSpan recursion.
func project(seq Sequence, pattern []string) []string {
	// Find the position where the pattern first completes in seq.Items
	// Greedy left-to-right: scan left to right, match greedily, stop at first completion
	var pos int
	matched := 0
	for i := 0; i < len(seq.Items) && matched < len(pattern); i++ {
		if seq.Items[i] == pattern[matched] {
			matched++
			pos = i
		}
	}

	// If pattern not fully matched, projected database is empty
	if matched < len(pattern) {
		return nil
	}

	// Return suffix after pos (earliest completion)
	if pos+1 < len(seq.Items) {
		return seq.Items[pos+1:]
	}
	return nil
}

// lexicographic returns a string for sorting patterns lexicographically
func lexicographic(items []string) string {
	var buf []byte
	for i, item := range items {
		if i > 0 {
			buf = append(buf, ' ')
		}
		buf = append(buf, []byte(item)...)
	}
	return string(buf)
}
