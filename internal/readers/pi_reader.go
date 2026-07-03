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

// PiReader implements SessionReader for Pi agent JSONL sessions.
// It captures text and toolCall inputs from `message` records, and tool
// results from separate `custom` records.
type PiReader struct{}

func (r *PiReader) Name() string { return "pi" }

func (r *PiReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFiles(def.Discover)
}

func (r *PiReader) Hash(path string) (string, error) {
	return hashfile.HashFile(path)
}

type piRecord struct {
	Type       string          `json:"type"`
	Timestamp  string          `json:"timestamp"`
	CWD        string          `json:"cwd"`
	CustomType string          `json:"customType"`
	Data       json.RawMessage `json:"data"`
	Message    *piMessage      `json:"message"`
}

type piMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type piBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Parse reads a Pi JSONL session and returns its messages as a ParsedFile.
// Only `message` records (text + toolCall) and `custom` records (tool results)
// produce messages; other record types are skipped.
func (r *PiReader) Parse(path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashfile.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}

	var msgs []models.Message
	var cwd string
	indexReasoning := def.Decode.IndexReasoning
	err = sync.IterateJSONLFile(path, func(_ int, line []byte) error {
		var rec piRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil // skip malformed lines
		}
		if cwd == "" && rec.CWD != "" {
			cwd = rec.CWD
		}
		switch rec.Type {
		case "message":
			msgs = append(msgs, extractPiMessages(rec, indexReasoning)...)
		case "custom":
			if m, ok := extractPiCustom(rec); ok {
				msgs = append(msgs, m)
			}
		}
		return nil
	})
	if err != nil {
		return models.ParsedFile{}, err
	}

	return models.ParsedFile{Path: path, Hash: hash, Records: msgs, Cwd: cwd}, nil
}

func piTimestamp(s string) time.Time {
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now()
	}
	return ts
}

func extractPiMessages(rec piRecord, indexReasoning bool) []models.Message {
	if rec.Message == nil {
		return nil
	}
	role := rec.Message.Role
	if role != "user" && role != "assistant" {
		return nil
	}
	ts := piTimestamp(rec.Timestamp)

	// content as a plain string
	var s string
	if err := json.Unmarshal(rec.Message.Content, &s); err == nil {
		text := sync.CleanContent(s)
		if text == "" {
			return nil
		}
		return []models.Message{{Role: role, Content: text, ContentType: classifyText(text), Timestamp: ts}}
	}

	// content as an array of blocks
	var blocks []piBlock
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
		case "toolCall":
			if t := SerializeToolInput(b.Name, b.Arguments); strings.TrimSpace(t) != "" {
				out = append(out, models.Message{Role: role, Content: t, ContentType: "tool", Timestamp: ts})
			}
		case "thinking":
			if indexReasoning {
				if m := extractPiReasoning(b, ts); m != nil {
					out = append(out, *m)
				}
			}
		}
	}
	if len(textParts) > 0 {
		text := strings.TrimSpace(strings.Join(textParts, " "))
		out = append([]models.Message{{Role: role, Content: text, ContentType: classifyText(text), Timestamp: ts}}, out...)
	}
	return out
}

// extractPiReasoning converts a thinking block into a searchable reasoning message.
func extractPiReasoning(block piBlock, ts time.Time) *models.Message {
	text := sync.CleanContent(block.Text)
	if text == "" {
		return nil
	}
	return &models.Message{
		Role:        "reasoning",
		Content:     text,
		ContentType: "reasoning",
		Timestamp:   ts,
	}
}

// extractPiCustom turns a Pi `custom` record (a tool result) into a searchable
// message. The result has no role of its own, so it is recorded as role "tool".
func extractPiCustom(rec piRecord) (models.Message, bool) {
	body := SerializeToolOutput(rec.Data)
	if strings.TrimSpace(body) == "" {
		return models.Message{}, false
	}
	if rec.CustomType != "" {
		body = rec.CustomType + " " + body
	}
	return models.Message{
		Role:        "tool",
		Content:     body,
		ContentType: "tool",
		Timestamp:   piTimestamp(rec.Timestamp),
	}, true
}
