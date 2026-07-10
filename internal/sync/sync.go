package sync

import "strings"

// IsNoiseType returns true if the type should be filtered
func IsNoiseType(typ string) bool {
	noiseTypes := map[string]bool{
		"system-reminder":      true,
		"task-notification":    true,
		"command":              true,
		"command-result":       true,
		"local-command-caveat": true,
		"progress":             true,
	}
	return noiseTypes[typ]
}

// CleanContent removes noise patterns from content
func CleanContent(content string) string {
	if content == "" {
		return ""
	}

	// Remove tags AND their contents (using regex-like patterns)
	// These tags should remove everything between opening and closing tags
	tagPatternsWithContent := []struct {
		open  string
		close string
	}{
		{"<system-reminder>", "</system-reminder>"},
		{"<task-notification>", "</task-notification>"},
		{"<caveat>", "</caveat>"},
		{"<local-command-caveat>", "</local-command-caveat>"},
		{"<command>", "</command>"},
		{"<local-command-stdout>", "</local-command-stdout>"},
		{"<command-name>", "</command-name>"},
		{"<command-message>", "</command-message>"},
		{"<command-args>", "</command-args>"},
		{"<teammate-message>", "</teammate-message>"},
	}

	for _, pattern := range tagPatternsWithContent {
		// Remove everything between tags including the tags
		for {
			start := strings.Index(content, pattern.open)
			if start == -1 {
				break
			}
			end := strings.Index(content[start:], pattern.close)
			if end == -1 {
				// Unmatched tag, just remove the opening tag
				content = content[:start] + content[start+len(pattern.open):]
				break
			}
			end += start // Adjust end to global position
			content = content[:start] + content[end+len(pattern.close):]
		}
	}

	// Remove "Caveat: " prefix and "Request interrupted" patterns
	content = strings.TrimPrefix(content, "Caveat: ")
	content = strings.ReplaceAll(content, "Request interrupted", "")

	// Clean extra whitespace
	content = strings.TrimSpace(content)
	// Collapse multiple spaces
	content = strings.Join(strings.Fields(content), " ")

	return content
}
