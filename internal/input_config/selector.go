package input_config

import (
	"strings"
)

// SelectField extracts a value from a record using a simple JSONPath-like selector.
// Supports: "$" (whole record), "$.field", "$.a.b" (nested), "$.a[0]" (array index).
// Returns (nil, false) for missing paths without panicking.
func SelectField(record map[string]any, path string) (any, bool) {
	if path == "$" || path == "" {
		return record, true
	}

	// Strip leading "$."
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")

	parts := splitPath(path)
	return traverse(record, parts)
}

// SelectString extracts a field as a string. Returns ("", false) if missing or not a string-like type.
func SelectString(record map[string]any, path string) (string, bool) {
	v, ok := SelectField(record, path)
	if !ok || v == nil {
		return "", false
	}
	s, ok := toString(v)
	return s, ok
}

func splitPath(path string) []string {
	// Convert bracket notation to dot notation: a[0] -> a.0
	path = strings.ReplaceAll(path, "[", ".")
	path = strings.ReplaceAll(path, "]", "")
	return strings.Split(path, ".")
}

func traverse(current any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return current, true
	}
	key := parts[0]
	rest := parts[1:]

	switch v := current.(type) {
	case map[string]any:
		child, ok := v[key]
		if !ok {
			return nil, false
		}
		return traverse(child, rest)
	case []any:
		// Wildcard: map over all elements
		if key == "*" {
			var out []any
			for _, elem := range v {
				val, ok := traverse(elem, rest)
				if ok {
					out = append(out, val)
				}
			}
			return out, len(out) > 0
		}
		// Numeric index
		idx := 0
		for _, c := range key {
			if c < '0' || c > '9' {
				return nil, false
			}
			idx = idx*10 + int(c-'0')
		}
		if idx >= len(v) {
			return nil, false
		}
		return traverse(v[idx], rest)
	}
	return nil, false
}
