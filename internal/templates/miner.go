package templates

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Template represents a discovered template (variable pattern).
type Template struct {
	Text                 string // normalized text with <*> for variables
	Signature            string // SHA256 hex of normalized text (deterministic identity)
	VariableCount        int    // count of <*> placeholders
	NormalizationVersion int    // versioning for future evolution
}

// Miner uses fixed-depth prefix clustering to extract error message templates.
// It normalizes tokens beyond a fixed depth to variables (<*>).
type Miner struct {
	depth int // fixed-depth prefix clustering (depth=2)
}

// NewMiner returns a miner with fixed-depth=2 prefix clustering.
func NewMiner() *Miner {
	return &Miner{depth: 2}
}

// ProcessLine extracts a template from a single line (e.g., an error message).
// Returns a Template with normalized text, signature, and variable count.
// Empty input returns empty Template.
func (m *Miner) ProcessLine(line string) Template {
	if strings.TrimSpace(line) == "" {
		return Template{}
	}
	tokens := strings.Fields(line)
	normalized := m.normalize(tokens)
	if normalized == "" {
		return Template{}
	}
	sig := m.signature(normalized)
	varCount := strings.Count(normalized, "<*>")
	return Template{
		Text:                 normalized,
		Signature:            sig,
		VariableCount:        varCount,
		NormalizationVersion: 1,
	}
}

// normalize applies Drain heuristics: convert variable (numeric, path-like) tokens to <*>.
// Keep first m.depth tokens verbatim; everything else is either <*> or kept based on heuristics.
func (m *Miner) normalize(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	// Keep the first m.depth tokens verbatim (they form the cluster key).
	// Beyond depth, heuristically identify variables.
	var result []string
	for i, tok := range tokens {
		if i < m.depth {
			// First m.depth tokens: keep verbatim (they are the "cluster signature").
			result = append(result, tok)
		} else {
			// Beyond depth: heuristic variable detection.
			if m.isVariable(tok) {
				result = append(result, "<*>")
			} else {
				result = append(result, tok)
			}
		}
	}
	return strings.Join(result, " ")
}

// isVariable detects if a token is likely a variable/value (numeric, quoted, path, etc.).
// Returns true for tokens that should become <*>.
func (m *Miner) isVariable(tok string) bool {
	// Numeric: 123, 0x1F, 1.23
	if isNumeric(tok) {
		return true
	}
	// Quoted string
	if (strings.HasPrefix(tok, `"`) && strings.HasSuffix(tok, `"`)) ||
		(strings.HasPrefix(tok, "'") && strings.HasSuffix(tok, "'")) {
		return true
	}
	// Path-like: /foo/bar, ./file, ~user
	if strings.Contains(tok, "/") || strings.Contains(tok, "\\") ||
		strings.HasPrefix(tok, ".") || strings.HasPrefix(tok, "~") {
		return true
	}
	// IP:port or host:port patterns (contains colon)
	if strings.Contains(tok, ":") {
		return true
	}
	// UUID/GUID-like: long hex or alphanumeric with hyphens
	if len(tok) >= 20 && (isHexLike(tok) || containsHyphens(tok)) {
		return true
	}
	return false
}

// isNumeric checks if a token is numeric (integer, float, hex).
func isNumeric(s string) bool {
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "+")
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		_, err := fmt.Sscanf(s, "%x", new(int64))
		return err == nil
	}
	for _, ch := range s {
		if !((ch >= '0' && ch <= '9') || ch == '.') {
			return false
		}
	}
	return s != "" && s != "."
}

// isHexLike checks if a token looks like a hex string.
func isHexLike(s string) bool {
	if len(s) < 6 {
		return false
	}
	for _, ch := range s {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

// containsHyphens checks if a token contains hyphens (common in IDs, UUIDs).
func containsHyphens(s string) bool {
	return strings.Contains(s, "-")
}

// signature returns the SHA256 hex hash of the normalized template text.
func (m *Miner) signature(text string) string {
	hash := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", hash)
}
