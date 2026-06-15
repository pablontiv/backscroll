package reader

import (
	"encoding/json"
	"fmt"
	"strings"

	internalsync "github.com/pablontiv/backscroll/internal/sync"
)

const semanticSnippetLimit = 500

// SemanticRow is a concise, agent-readable row extracted from a JSONL input file.
type SemanticRow struct {
	Path, Timestamp, Role, Kind, Content string
	Line, Ordinal                        int
}

// ReadSemanticTail returns the last tail semantic rows from path.
func ReadSemanticTail(path string, tail int) ([]SemanticRow, error) {
	if tail < 0 {
		return nil, fmt.Errorf("tail must be non-negative")
	}
	var all, ring []SemanticRow
	if tail > 0 {
		ring = make([]SemanticRow, 0, tail)
	}
	seen := 0
	err := internalsync.IterateJSONLFile(path, func(lineNumber int, line []byte) error {
		for _, row := range semanticRowsFromLine(path, lineNumber, line) {
			seen++
			row.Ordinal = seen
			if tail <= 0 {
				all = append(all, row)
			} else if len(ring) < tail {
				ring = append(ring, row)
			} else {
				ring[(seen-1)%tail] = row
			}
		}
		return nil
	})
	if err != nil || tail <= 0 || len(ring) < tail {
		return append(all, ring...), err
	}
	rows := make([]SemanticRow, 0, len(ring))
	for i, start := 0, seen%tail; i < len(ring); i++ {
		rows = append(rows, ring[(start+i)%tail])
	}
	return rows, nil
}

func semanticRowsFromLine(path string, lineNumber int, line []byte) []SemanticRow {
	var rec map[string]any
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil
	}
	msg, ok := rec["message"].(map[string]any)
	if !ok {
		return nil
	}
	base := SemanticRow{Path: path, Line: lineNumber, Timestamp: stringField(rec, "timestamp"), Role: stringField(msg, "role")}
	if text, ok := msg["content"].(string); ok {
		return semanticTextRow(base, text, semanticTextKind(base))
	}
	blocks, ok := msg["content"].([]any)
	if !ok {
		return nil
	}
	rows := make([]SemanticRow, 0, len(blocks))
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		row := base
		switch stringField(block, "type") {
		case "text":
			row.Kind, row.Content = semanticTextKind(base), truncateSnippet(stringField(block, "text"))
		case "tool_use", "toolCall":
			row.Kind = "tool_use"
			row.Content = truncateSnippet(toolSnippet(stringField(block, "name"), stringField(block, "id"), jsonField(block, "input", "arguments", "toolCall")))
		case "tool_result", "toolResult":
			row.Kind = "tool_result"
			row.Content = truncateSnippet(toolSnippet("", firstNonEmpty(stringField(block, "tool_use_id"), stringField(block, "toolCallId")), jsonField(block, "content", "toolResult")))
		}
		if row.Kind != "" && row.Content != "" {
			rows = append(rows, row)
		}
	}
	return rows
}

func semanticTextRow(base SemanticRow, text string, kind string) []SemanticRow {
	if text = truncateSnippet(text); text == "" {
		return nil
	}
	base.Kind, base.Content = kind, text
	return []SemanticRow{base}
}

func semanticTextKind(base SemanticRow) string {
	if base.Role == "toolResult" {
		return "tool_result"
	}
	return "text"
}

func stringField(m map[string]any, key string) string {
	if value, ok := m[key].(string); ok {
		return value
	}
	return ""
}

func jsonField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil {
			if encoded, err := json.Marshal(value); err == nil && string(encoded) != "null" {
				return string(encoded)
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func toolSnippet(name string, id string, payload string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{"name=" + name, "id=" + id, "payload=" + payload} {
		if !strings.HasSuffix(part, "=") {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " ")
}

func truncateSnippet(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	if len(content) <= semanticSnippetLimit {
		return content
	}
	return content[:semanticSnippetLimit] + "…"
}
