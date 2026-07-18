package projects

import (
	"os"
	"path/filepath"
	"strings"
)

// DecodeCwdFromSessionPath attempts to decode a Claude session path to recover the cwd.
// Claude encodes session paths as: /Users/pones/.claude/projects/-Users-Shared-harness-backscroll/uuid.jsonl
// where the directory component after /projects/ uses dashes for slashes: -Users-Shared-harness-backscroll
// represents /Users/Shared/harness/backscroll.
//
// Strategy: extract the directory component, decode dashes to slashes, check if the resulting
// path exists on disk → use its basename; else fallback to the last dash-separated segment.
// Returns empty string if the path is malformed or unparseable.
func DecodeCwdFromSessionPath(sourcePath string) string {
	// Check if sourcePath contains "/.claude/projects/"
	const marker = "/.claude/projects/"
	idx := strings.Index(sourcePath, marker)
	if idx == -1 {
		return ""
	}

	// Extract everything after the marker
	afterMarker := sourcePath[idx+len(marker):]

	// Find the next slash (directory name)
	nextSlash := strings.Index(afterMarker, "/")
	if nextSlash == -1 || nextSlash == 0 {
		return ""
	}

	encodedDir := afterMarker[:nextSlash]

	// Try to decode: replace dashes with slashes, but strip leading/trailing dashes first
	decoded := decodeEncodedPath(encodedDir)
	if decoded == "" {
		return ""
	}

	// Check if decoded path exists on disk; if so, use its basename
	if _, err := os.Stat(decoded); err == nil {
		return filepath.Base(decoded)
	}

	// Fallback: use the last dash-separated segment as the project basename
	parts := strings.Split(encodedDir, "-")
	if len(parts) > 0 {
		// Find the last non-empty part
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" {
				return parts[i]
			}
		}
	}

	return ""
}

// decodeEncodedPath converts a dash-encoded path back to slashes.
// Example: "-Users-Shared-harness-backscroll" → "/Users/Shared/harness/backscroll" (if it exists)
// The encoding assumes a single leading dash, then interior dashes map to slashes.
func decodeEncodedPath(encoded string) string {
	// Strip leading/trailing dashes
	trimmed := strings.Trim(encoded, "-")
	if trimmed == "" {
		return ""
	}

	// Replace interior dashes with slashes
	// Note: this is a heuristic and cannot perfectly decode if the original path had dashes.
	// Example: "-my-project-name" could decode to "/my/project/name" or be a single component "my-project-name".
	// We use the exists-check as a disambiguator.
	decoded := "/" + strings.ReplaceAll(trimmed, "-", "/")
	return decoded
}
