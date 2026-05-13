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

// Tag returns all matching category tags for the given text content.
// It searches the content against all category patterns and returns
// the names of categories that match.
func Tag(content string) []string {
	// Normalize to lowercase for comparison
	lowerContent := strings.ToLower(content)

	var tags []string
	for category, pattern := range patterns {
		if pattern.MatchString(lowerContent) {
			tags = append(tags, category)
		}
	}

	return tags
}
