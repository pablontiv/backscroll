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

// CodexReader reads Codex CLI rollout JSONL. Only response_item records are
// searchable: event_msg and compaction histories can duplicate that stream.
// See docs/research/codex-input-evidence.md for the observed format boundary.
type CodexReader struct{}

func (*CodexReader) Name() string { return "codex" }

func (*CodexReader) Discover(def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFiles(def.Discover)
}

func (*CodexReader) Hash(path string) (string, error) { return hashfile.HashFile(path) }

type codexRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type codexItem struct {
	Type      string          `json:"type"`
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	Summary   json.RawMessage `json:"summary"`
	Name      string          `json:"name"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	Output    json.RawMessage `json:"output"`
}

// Parse skips malformed/unknown records without serializing their raw payload.
// Missing or invalid response timestamps are skipped instead of assigning now,
// keeping re-sync deterministic. Like Pi/OpenCode, records use the existing
// UUID-less per-file sync path; Codex item IDs are not universally present.
func (*CodexReader) Parse(path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashfile.HashFile(path)
	if err != nil {
		return models.ParsedFile{}, err
	}
	result := models.ParsedFile{Path: path, Hash: hash}
	err = sync.IterateJSONLFile(path, func(_ int, line []byte) error {
		var rec codexRecord
		if json.Unmarshal(line, &rec) != nil {
			return nil
		}
		switch rec.Type {
		case "session_meta":
			var meta struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal(rec.Payload, &meta) == nil && result.Cwd == "" {
				result.Cwd = meta.Cwd
			}
		case "response_item":
			ts, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
			if err != nil {
				return nil
			}
			var item codexItem
			if json.Unmarshal(rec.Payload, &item) != nil {
				return nil
			}
			if msg, ok := codexMessage(item, ts, def.Decode.IndexReasoning); ok {
				result.Records = append(result.Records, msg)
			}
		}
		return nil
	})
	if err != nil {
		return models.ParsedFile{}, err
	}
	return result, nil
}

func codexMessage(item codexItem, ts time.Time, reasoning bool) (models.Message, bool) {
	msg := models.Message{Timestamp: ts}
	switch item.Type {
	case "message":
		if item.Role != "user" && item.Role != "assistant" {
			return msg, false
		}
		msg.Role = item.Role
		parts := codexTextParts(item.Content, "input_text", "output_text")
		if item.Role == "user" {
			for i, text := range parts {
				parts[i] = stripCodexInjectedWrappers(text)
			}
		}
		msg.Content = sync.CleanContent(strings.Join(parts, "\n"))
		msg.ContentType = classifyText(msg.Content)
	case "function_call":
		// Codex stores arguments as JSON encoded inside a JSON string.
		var args map[string]json.RawMessage
		if strings.TrimSpace(item.Name) == "" || json.Unmarshal([]byte(item.Arguments), &args) != nil || args == nil {
			return msg, false
		}
		msg.Role, msg.ContentType = "assistant", "tool"
		msg.Content = SerializeToolInput(item.Name, json.RawMessage(item.Arguments))
	case "custom_tool_call":
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Input) == "" {
			return msg, false
		}
		raw, _ := json.Marshal(item.Input)
		msg.Role, msg.ContentType = "assistant", "tool"
		msg.Content = SerializeToolInput(item.Name, raw)
	case "function_call_output", "custom_tool_call_output":
		var text string
		if json.Unmarshal(item.Output, &text) != nil {
			text = codexTextBlocks(item.Output, "input_text", "output_text")
		}
		// Never serialize unknown output objects or image/audio payloads into FTS.
		raw, _ := json.Marshal(text)
		msg.Role, msg.ContentType = "tool", "tool"
		msg.Content = SerializeToolOutput(raw)
	case "reasoning":
		if !reasoning {
			return msg, false
		}
		msg.Role, msg.ContentType = "reasoning", "reasoning"
		msg.Content = sync.CleanContent(strings.Join([]string{
			codexTextBlocks(item.Summary, "summary_text"),
			codexTextBlocks(item.Content, "reasoning_text", "text"),
		}, " "))
	default:
		return msg, false
	}
	return msg, strings.TrimSpace(msg.Content) != ""
}

// codexTextBlocks selects only explicit text types, excluding arbitrary JSON
// and multimodal data. A malformed block does not hide valid sibling blocks.
func codexTextBlocks(raw json.RawMessage, types ...string) string {
	return strings.Join(codexTextParts(raw, types...), "\n")
}

func codexTextParts(raw json.RawMessage, types ...string) []string {
	var blocks []json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var parts []string
	for _, rawBlock := range blocks {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(rawBlock, &block) != nil {
			continue
		}
		for _, typ := range types {
			if block.Type == typ && strings.TrimSpace(block.Text) != "" {
				parts = append(parts, block.Text)
				break
			}
		}
	}
	return parts
}

// stripCodexInjectedWrappers removes only complete, leading known injection
// pairs from a user text block. Keep task/unknown wrappers, embedded examples,
// incomplete pairs and prose after the closing tag. Never apply this policy to
// assistant messages, reasoning or tool output; their text is recall evidence.
func stripCodexInjectedWrappers(text string) string {
	text = strings.TrimSpace(text)
	for {
		removed := false
		for _, tag := range [...]string{"recommended_plugins", "environment_context", "heartbeat", "turn_aborted"} {
			open, close := "<"+tag+">", "</"+tag+">"
			if !strings.HasPrefix(text, open) {
				continue
			}
			end := strings.Index(text[len(open):], close)
			if end < 0 {
				return text
			}
			text = strings.TrimSpace(text[len(open)+end+len(close):])
			removed = true
			break
		}
		if !removed {
			return text
		}
	}
}
