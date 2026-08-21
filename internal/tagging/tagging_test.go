package tagging

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestTag(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "debugging",
			content:  "Fixed a critical bug in error handling with panic recovery",
			expected: []string{"debugging"},
		},
		{
			name:     "refactoring",
			content:  "Refactor and restructure the codebase for better cleanup",
			expected: []string{"refactoring"},
		},
		{
			name:     "feature",
			content:  "Implement a new feature to build a custom component",
			expected: []string{"feature"},
		},
		{
			name:     "testing",
			content:  "Add unit tests and integration specs with mock objects and coverage analysis",
			expected: []string{"testing"},
		},
		{
			name:     "docs",
			content:  "Document the README with comments and explanations",
			expected: []string{"docs"},
		},
		{
			name:     "config",
			content:  "Configure CI/CD setup with YAML and TOML files for deployment",
			expected: []string{"config"},
		},
		{
			name:     "case insensitive",
			content:  "BUG: Error in PANIC handling",
			expected: []string{"debugging"},
		},
		{
			name:     "multiple categories",
			content:  "Fix bug by refactoring tests with documentation",
			expected: []string{"debugging", "docs", "refactoring", "testing"},
		},
		{
			name:     "no matches",
			content:  "Random text without any relevant keywords",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Tag(tt.content)
			sort.Strings(result)
			sort.Strings(tt.expected)

			if len(result) != len(tt.expected) {
				t.Errorf("got %d tags, expected %d", len(result), len(tt.expected))
				t.Errorf("got: %v, expected: %v", result, tt.expected)
				return
			}

			for i, tag := range result {
				if tag != tt.expected[i] {
					t.Errorf("tag mismatch at index %d: got %q, expected %q", i, tag, tt.expected[i])
				}
			}
		})
	}
}

func TestTagDetectsSixCategories(t *testing.T) {
	// Verify all 6 categories can be detected
	testCases := map[string]string{
		"debugging":   "This is a bug that causes a panic",
		"refactoring": "Time to refactor this code",
		"feature":     "Implement this new feature",
		"testing":     "We need better test coverage",
		"docs":        "Document this in the README",
		"config":      "Configure the CI pipeline",
	}

	for category, content := range testCases {
		result := Tag(content)
		found := false
		for _, tag := range result {
			if tag == category {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("category %q not detected in: %s", category, content)
		}
	}
}

func TestAccumulatorMatchesJoinedSessionTagSet(t *testing.T) {
	cases := [][]string{
		nil,
		{"BUG in parser", "Document the README"},
		{"clean", "up does not cross the newline"},
		{"Implement feature", "add tests", "configure CI"},
		{"", "Random text", "PANIC"},
	}
	for _, messages := range cases {
		var acc Accumulator
		for _, message := range messages {
			acc.Add(message)
		}
		got := acc.Tags()
		want := Tag(strings.Join(messages, "\n"))
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("messages=%q tags=%v want=%v", messages, got, want)
		}
	}
}

func TestAccumulatorTagsReturnsIndependentSlice(t *testing.T) {
	var acc Accumulator
	acc.Add("fix bug and add tests")
	first := acc.Tags()
	first[0] = "mutated"
	second := acc.Tags()
	if slices.Contains(second, "mutated") {
		t.Fatalf("Tags returned aliased state: %v", second)
	}
}

func BenchmarkAccumulatorScaling(b *testing.B) {
	base := strings.Repeat("ordinary prose with a final bug marker ", 256)
	for _, records := range []int{128, 256, 512} {
		b.Run(fmt.Sprintf("records_%d", records), func(b *testing.B) {
			messages := make([]string, records)
			for i := range messages {
				messages[i] = base
			}
			b.ReportAllocs()
			b.SetBytes(int64(records * len(base)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var acc Accumulator
				for _, message := range messages {
					acc.Add(message)
				}
				_ = acc.Tags()
			}
		})
	}
}
