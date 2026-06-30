package readers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/sync"
	"github.com/pablontiv/picokit/hashfile"
)

// ClaudeReader implements SessionReader for Claude Code JSONL sessions.
// It captures text, tool_use inputs, and tool_result outputs (including errors).
type ClaudeReader struct{}

func (r *ClaudeReader) Name() string { return "claude" }

func (r *ClaudeReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFiles(def.Discover)
}

func (r *ClaudeReader) Hash(path string) (string, error) {
	return hashfile.HashFile(path)
}

type claudeRecord struct {
	Type      string         `json:"type"`
	Timestamp string         `json:"timestamp"`
	CWD       string         `json:"cwd"`
	IsMeta    bool           `json:"isMeta"`
	Message   *claudeMessage `json:"message"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Name    string          `json:"name"`
	Input   json.RawMessage `json:"input"`
	Content json.RawMessage `json:"content"`
	IsError bool            `json:"is_error"`
}

// Parse reads a Claude JSONL session and returns its messages as a ParsedFile.
// One record may yield several messages: one for concatenated text plus one per
// tool_use / tool_result block (so each tool call is independently searchable).
func (r *ClaudeReader) Parse(path string, _ input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashfile.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	var msgs []models.Message
	var cwd string
	err = sync.IterateJSONLFile(path, func(_ int, line []byte) error {
		var rec claudeRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil // skip malformed lines
		}
		if cwd == "" && rec.CWD != "" {
			cwd = rec.CWD
		}
		if rec.IsMeta || sync.IsNoiseType(rec.Type) || rec.Message == nil || rec.Message.Role == "" {
			return nil
		}
		msgs = append(msgs, extractClaudeMessages(rec)...)
		return nil
	})
	if err != nil {
		return models.ParsedFile{}, err
	}

	return models.ParsedFile{Path: path, Hash: hash, Records: msgs, Cwd: cwd}, nil
}

func extractClaudeMessages(rec claudeRecord) []models.Message {
	ts, err := time.Parse(time.RFC3339, rec.Timestamp)
	if err != nil {
		ts = time.Now()
	}
	role := rec.Message.Role

	// content as a plain string
	var s string
	if err := json.Unmarshal(rec.Message.Content, &s); err == nil {
		text := sync.CleanContent(s)
		if text == "" {
			return nil
		}
		return []models.Message{{Role: role, Content: text, ContentType: classifyClaude(text), Timestamp: ts}}
	}

	// content as an array of blocks
	var blocks []claudeBlock
	if err := json.Unmarshal(rec.Message.Content, &blocks); err != nil {
		return nil
	}
	var out []models.Message
	var textParts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if c := sync.CleanContent(b.Text); c != "" {
				textParts = append(textParts, c)
			}
		case "tool_use":
			if t := SerializeToolInput(b.Name, b.Input); strings.TrimSpace(t) != "" {
				out = append(out, models.Message{Role: role, Content: t, ContentType: "tool", Timestamp: ts})
			}
		case "tool_result":
			body := SerializeToolOutput(b.Content)
			if b.IsError {
				body = "error: " + body
			}
			if strings.TrimSpace(body) != "" {
				out = append(out, models.Message{Role: role, Content: body, ContentType: "tool", Timestamp: ts})
			}
		}
	}
	if len(textParts) > 0 {
		text := strings.TrimSpace(strings.Join(textParts, " "))
		out = append([]models.Message{{Role: role, Content: text, ContentType: classifyClaude(text), Timestamp: ts}}, out...)
	}
	return out
}

func classifyClaude(text string) string {
	if strings.Contains(text, "```") {
		return "code"
	}
	return "text"
}
