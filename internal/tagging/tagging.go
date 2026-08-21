package tagging

import (
	"regexp"
	"strings"
)

var patterns = map[string]*regexp.Regexp{
	"debugging":   regexp.MustCompile(`(?i)(error|bug|fix|debug|panic|crash|traceback|exception)`),
	"refactoring": regexp.MustCompile(`(?i)(refactor|restructur|reorganiz|cleanup|clean up|rewrite)`),
	"feature":     regexp.MustCompile(`(?i)(implement|add feature|new feature|create|build|develop)`),
	"testing":     regexp.MustCompile(`(?i)(test|spec|assert|mock|coverage|jest|pytest)`),
	"docs":        regexp.MustCompile(`(?i)(document|readme|comment|explain|description)`),
	"config":      regexp.MustCompile(`(?i)(config|setup|configure|install|deploy|ci|yaml|toml)`),
}

// Accumulator incrementally collects tags across multiple content chunks.
type Accumulator struct {
	matched map[string]struct{}
}

// Add adds content to the accumulator and records any matching tags.
func (a *Accumulator) Add(content string) {
	lowerContent := strings.ToLower(content)
	for category, pattern := range patterns {
		if pattern.MatchString(lowerContent) {
			if a.matched == nil {
				a.matched = make(map[string]struct{}, len(patterns))
			}
			a.matched[category] = struct{}{}
		}
	}
}

// Tags returns the accumulated category tags.
func (a *Accumulator) Tags() []string {
	tags := make([]string, 0, len(a.matched))
	for category := range a.matched {
		tags = append(tags, category)
	}
	return tags
}

// Tag returns all matching category tags for the given text content.
// It searches the content against all category patterns and returns
// the names of categories that match.
func Tag(content string) []string {
	var accumulator Accumulator
	accumulator.Add(content)
	return accumulator.Tags()
}
