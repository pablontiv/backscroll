package sync

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

// ParseSessions parses a JSONL session file and returns Message records.
// It defensively handles both Claude and Pi formats.
func ParseSessions(path string) ([]models.Message, error) {
	var messages []models.Message
	err := IterateJSONLFile(path, func(_ int, line []byte) error {
		var rawRec rawRecord
		if err := json.Unmarshal(line, &rawRec); err != nil {
			// Skip malformed lines
			return nil
		}

		// Skip noise records
		if IsNoiseRecord(rawRec) {
			return nil
		}

		// Extract message if present
		if rawRec.Message == nil {
			return nil
		}

		// Parse timestamp
		ts, err := time.Parse(time.RFC3339, rawRec.Timestamp)
		if err != nil {
			ts = time.Now()
		}

		// Extract content and content type
		content, contentType := extractContent(rawRec.Message.Content)
		if content == "" {
			return nil
		}

		messages = append(messages, models.Message{
			Role:        rawRec.Message.Role,
			Content:     content,
			ContentType: contentType,
			Timestamp:   ts,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan session file: %w", err)
	}

	return messages, nil
}

// WalkSessionDirs traverses session directories and returns parseable JSONL paths.
// If includeAgents is false, subagent sessions are skipped.
func WalkSessionDirs(dirs []string, includeAgents bool) ([]string, error) {
	var jsonlPaths []string

	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Skip inaccessible directories
				return nil
			}

			// Skip subagent directories unless explicitly included
			if !includeAgents && strings.Contains(path, string(filepath.Separator)+"subagents"+string(filepath.Separator)) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Collect .jsonl files
			if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
				jsonlPaths = append(jsonlPaths, path)
			}

			return nil
		})

		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("walk directory %s: %w", dir, err)
		}
	}

	return jsonlPaths, nil
}

// IsNoiseRecord returns true if the record should be filtered out.
func IsNoiseRecord(r rawRecord) bool {
	// Filter by type
	if isNoiseType(r.Type) {
		return true
	}

	// Filter metadata records
	if r.IsMeta {
		return true
	}

	// Filter records with empty role
	if r.Message == nil || r.Message.Role == "" {
		return true
	}

	// Filter records where content extraction yields nothing
	content, _ := extractContent(r.Message.Content)
	return content == ""
}

// rawRecord covers both Claude and Pi formats defensively
type rawRecord struct {
	Type      string      `json:"type"`
	UUID      string      `json:"uuid"`
	ID        string      `json:"id"`
	SessionID string      `json:"sessionId"`
	ParentID  string      `json:"parentId"`
	Timestamp string      `json:"timestamp"`
	CWD       string      `json:"cwd"`
	IsAgent   bool        `json:"isAgent"`
	IsMeta    bool        `json:"isMeta"`
	Message   *rawMessage `json:"message"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type rawBlock struct {
	Type       string          `json:"type"`
	Text       string          `json:"text"`
	Thinking   string          `json:"thinking"`
	ToolCall   json.RawMessage `json:"toolCall"`
	ToolResult json.RawMessage `json:"toolResult"`
	ToolCallID string          `json:"toolCallId"`
	ToolUseID  string          `json:"tool_use_id"`
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Input      json.RawMessage `json:"input"`
	Arguments  json.RawMessage `json:"arguments"`
	Content    json.RawMessage `json:"content"`
}

// isNoiseType returns true if the type should be filtered
func isNoiseType(typ string) bool {
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

// extractContent extracts text from either a string or array of blocks.
// Returns (content, contentType).
func extractContent(raw json.RawMessage) (string, string) {
	if raw == nil {
		return "", "text"
	}

	// Try parsing as string first
	var strContent string
	if err := json.Unmarshal(raw, &strContent); err == nil {
		content := cleanContent(strContent)
		return content, classifyContentType(content, false)
	}

	// Try parsing as array of blocks
	var blocks []rawBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return extractFromBlocks(blocks)
	}

	return "", "text"
}

// extractFromBlocks extracts content and determines type from an array of blocks
func extractFromBlocks(blocks []rawBlock) (string, string) {
	var textParts []string
	hasToolCall := false
	hasCode := false

	for _, block := range blocks {
		switch block.Type {
		case "text":
			cleaned := cleanContent(block.Text)
			if cleaned != "" {
				textParts = append(textParts, cleaned)
				if strings.Contains(cleaned, "```") {
					hasCode = true
				}
			}
		case "thinking":
			// Ignore thinking blocks (hidden reasoning)
		case "tool_use", "toolCall":
			hasToolCall = true
		case "tool_result", "toolResult":
			// Ignore tool result blocks in message content
		case "tool_use_id":
			hasToolCall = true
		}
	}

	content := strings.TrimSpace(strings.Join(textParts, " "))
	if content == "" {
		return "", "text"
	}

	// Determine content type
	contentType := "text"
	if hasCode {
		contentType = "code"
	} else if hasToolCall {
		contentType = "tool"
	}

	return content, contentType
}

// cleanContent removes noise patterns from content
func cleanContent(content string) string {
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

// classifyContentType determines content type from text
func classifyContentType(content string, hasCode bool) string {
	if hasCode {
		return "code"
	}
	if strings.Contains(content, "```") {
		return "code"
	}
	return "text"
}
