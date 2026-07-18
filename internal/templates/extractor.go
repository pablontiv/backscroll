package templates

import (
	"regexp"
	"strings"
)

// ExtractErrorLines selects lines from a tool output that should be mined for templates.
// Strategy is calibrated per tool_name to handle real output structure
// (e.g., Go compiler errors lead; test FAILs trail; panics sit mid-output).
// Returns only non-empty lines.
func ExtractErrorLines(toolName string, text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	lines := strings.Split(text, "\n")
	var relevant []string

	switch toolName {
	case "Bash":
		// Bash: prefer last non-empty line + any line with error/fail/panic/denied/fatal.
		// This handles both "exit code 1" cases (signal in last line) and
		// explicit error output.
		relevant = extractBashErrors(lines)
	case "Go":
		// Go compiler/test: first error line, any FAIL line.
		relevant = extractGoErrors(lines)
	default:
		// Fallback: take last non-empty line + error-matching lines.
		relevant = extractGenericErrors(lines)
	}

	return relevant
}

// extractBashErrors returns the last non-empty line plus any error/fail/panic line.
func extractBashErrors(lines []string) []string {
	var result []string
	var lastNonEmpty string
	errorPattern := regexp.MustCompile(`(?i)(error|fail|panic|denied|fatal)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lastNonEmpty = line
		if errorPattern.MatchString(line) {
			result = append(result, line)
		}
	}

	// Deduplicate: if lastNonEmpty already in result, don't add twice.
	if lastNonEmpty != "" {
		found := false
		for _, r := range result {
			if r == lastNonEmpty {
				found = true
				break
			}
		}
		if !found {
			result = append(result, lastNonEmpty)
		}
	}

	return result
}

// extractGoErrors returns the first error line and any FAIL lines.
func extractGoErrors(lines []string) []string {
	var result []string
	errorPattern := regexp.MustCompile(`^(.*?):\s*error:|FAIL\s+|panic\(`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if errorPattern.MatchString(line) {
			result = append(result, line)
		}
	}
	return result
}

// extractGenericErrors returns the last non-empty line plus error-matching lines.
func extractGenericErrors(lines []string) []string {
	var result []string
	var lastNonEmpty string
	errorPattern := regexp.MustCompile(`(?i)(error|fail|panic|denied|fatal)`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lastNonEmpty = line
		if errorPattern.MatchString(line) {
			result = append(result, line)
		}
	}

	if lastNonEmpty != "" {
		found := false
		for _, r := range result {
			if r == lastNonEmpty {
				found = true
				break
			}
		}
		if !found {
			result = append(result, lastNonEmpty)
		}
	}

	return result
}
