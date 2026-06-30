package readers

import (
	"encoding/json"
	"sort"
	"strings"
)

// MaxToolTextLen caps the searchable text extracted from a single tool input or
// output. Chosen from observed live-session sizes (p90 ~4000 chars); caps the
// rare ~57KB outlier so the FTS index stays lean.
const MaxToolTextLen = 4000

// SerializeToolInput turns a tool name and its input value into searchable text,
// e.g. `bash command=... description=...`. Objects become space-joined key=value
// pairs (keys sorted for determinism); other shapes fall back to compact JSON.
// The result is truncated to MaxToolTextLen runes.
func SerializeToolInput(name string, input json.RawMessage) string {
	out := strings.TrimSpace(name + " " + serializeValue(input))
	return truncateRunes(out, MaxToolTextLen)
}

// SerializeToolOutput turns a tool result value into searchable text. Strings pass
// through; arrays of {type:text} blocks are joined; other shapes become compact
// JSON. The result is truncated to MaxToolTextLen runes.
func SerializeToolOutput(content json.RawMessage) string {
	return truncateRunes(serializeValue(content), MaxToolTextLen)
}

func serializeValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+scalar(obj[k]))
		}
		return strings.Join(parts, " ")
	}
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		parts := make([]string, 0, len(arr))
		for _, el := range arr {
			if t, ok := el["text"]; ok {
				parts = append(parts, scalar(t))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return string(raw)
}

// scalar renders a value as plain text: strings unquoted, everything else compact JSON.
func scalar(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
