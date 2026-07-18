package sync

import (
	"fmt"
	"regexp"
)

var (
	// exitCodePattern matches common exit-code output formats from Bash-like tools.
	// Patterns: "exit code N", "Exit code: N", "returned N" (bare "exit: N" is NOT matched)
	// Captures only the first match; only Bash tool results are mined.
	exitCodePattern = regexp.MustCompile(`(?i)(?:exit\s+code|returned)\s*[:=]?\s*(\d+)`)
)

// ExtractExitCode parses exit-code patterns from tool-result text.
// Returns *int if matched and tool is Bash; otherwise nil.
// Pattern matching is case-insensitive to accommodate variation.
func ExtractExitCode(text, toolName string) *int {
	if toolName != "Bash" {
		return nil
	}
	if text == "" {
		return nil
	}

	matches := exitCodePattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return nil
	}

	// matches[1] is the captured code (first match only)
	var code int
	if _, err := fmt.Sscanf(matches[1], "%d", &code); err != nil {
		return nil
	}
	return &code
}
