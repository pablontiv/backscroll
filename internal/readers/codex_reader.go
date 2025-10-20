package readers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/directsearch"
	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/sync"
)

// CodexReader reads Codex CLI rollout JSONL. Only response_item records are
// searchable: event_msg and compaction histories can duplicate that stream.
// See docs/research/codex-input-evidence.md for the observed format boundary.
type CodexReader struct{}

func (*CodexReader) Name() string           { return "codex" }
func (*CodexReader) IngestionEpoch() uint64 { return 2 }

func (*CodexReader) Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFilesContext(ctx, def.Discover)
}

func (*CodexReader) Hash(ctx context.Context, path string) (string, error) {
	return hashFile(ctx, path)
}

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
	CallID    string          `json:"call_id"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	Output    json.RawMessage `json:"output"`
	IsError   *bool           `json:"is_error"`
}

// Call linkage and raw result proof are independent of searchable output text:
// an empty or unsupported output can still carry native outcome metadata.
type codexResult struct {
	callID string
	item   codexItem
	text   string
	index  int
}

// Parse skips malformed/unknown records without serializing their raw payload.
// Missing or invalid response timestamps are skipped instead of assigning now,
// keeping re-sync deterministic. Records retain the UUID-less source-snapshot
// lifecycle because native item IDs are not universally present.
func (*CodexReader) Parse(ctx context.Context, path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashFile(ctx, path)
	if err != nil {
		return models.ParsedFile{}, err
	}
	result := models.ParsedFile{Path: path, Hash: hash}
	calls := make(map[string]int)
	var outputs []codexResult
	err = sync.IterateJSONLFileContext(ctx, path, func(_ int, line []byte) error {
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
			var msg models.Message
			var ok bool
			var outputText string
			if item.Type == "function_call_output" || item.Type == "custom_tool_call_output" {
				outputText = codexOutputText(ctx, item.Output)
				msg, ok = codexOutputMessage(item, ts, outputText)
			} else {
				msg, ok = codexMessage(ctx, item, ts, def.Decode.IndexReasoning)
			}
			index := -1
			if ok {
				index = len(result.Records)
				result.Records = append(result.Records, msg)
				if (item.Type == "function_call" || item.Type == "custom_tool_call") && item.CallID != "" {
					calls[item.CallID] = index
				}
			}
			if item.Type == "function_call_output" || item.Type == "custom_tool_call_output" {
				outputs = append(outputs, codexResult{callID: item.CallID, item: item, text: outputText, index: index})
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return models.ParsedFile{}, err
	}
	// Pair by native identity across interleaving, including outputs that precede
	// calls. Results never acquire ToolName and therefore never become events.
	for _, output := range outputs {
		if err := ctx.Err(); err != nil {
			return models.ParsedFile{}, err
		}
		j, ok := calls[output.callID]
		if !ok || output.callID == "" {
			continue
		}
		call := &result.Records[j]
		isError, exitCode := codexOutcome(call.ToolName, output.item, output.text)
		if call.IsError == nil {
			call.IsError = isError
		}
		if call.ExitCode == nil {
			call.ExitCode = exitCode
		}
		if output.index >= 0 {
			msg := &result.Records[output.index]
			msg.SearchEcho, msg.IsError, msg.ExitCode = call.SearchEcho, isError, exitCode
		}
	}
	return result, nil
}

// isCodexDirectSearchCall is now a thin wrapper around the shared predicate
// in internal/directsearch. The storage replay path uses the same function,
// so the two call sites cannot drift apart.
func isCodexDirectSearchCall(tool, arguments string) bool {
	return directsearch.IsCodexDirectSearchCall(tool, arguments)
}

func codexMessage(ctx context.Context, item codexItem, ts time.Time, reasoning bool) (models.Message, bool) {
	if ctx.Err() != nil {
		return models.Message{}, false
	}
	msg := models.Message{Timestamp: ts}
	switch item.Type {
	case "message":
		if item.Role != "user" && item.Role != "assistant" {
			return msg, false
		}
		msg.Role = item.Role
		msg.Origin = messageOriginForRole(item.Role)
		parts := codexTextParts(ctx, item.Content, "input_text", "output_text")
		if item.Role == "user" {
			for i, text := range parts {
				if ctx.Err() != nil {
					return msg, false
				}
				parts[i] = stripCodexInjectedWrappers(text)
			}
		}
		msg.Content, msg.ContentType = cleanText(strings.Join(parts, "\n"))
	case "function_call":
		// Codex stores arguments as JSON encoded inside a JSON string.
		var args map[string]json.RawMessage
		if strings.TrimSpace(item.Name) == "" || json.Unmarshal([]byte(item.Arguments), &args) != nil || args == nil {
			return msg, false
		}
		msg.Role, msg.Origin, msg.ContentType = "assistant", models.OriginAssistant, "tool"
		msg.Content = SerializeToolInput(item.Name, json.RawMessage(item.Arguments))
		msg.SearchEcho = isCodexDirectSearchCall(item.Name, item.Arguments)
		msg.ToolName, msg.ToolUseID = item.Name, item.CallID
		msg.CommandHead = codexCommandHead(item.Name, item.Arguments)
	case "custom_tool_call":
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Input) == "" {
			return msg, false
		}
		raw, _ := json.Marshal(item.Input)
		msg.Role, msg.Origin, msg.ContentType = "assistant", models.OriginAssistant, "tool"
		msg.Content = SerializeToolInput(item.Name, raw)
		msg.ToolName, msg.ToolUseID = item.Name, item.CallID
	case "function_call_output", "custom_tool_call_output":
		return codexOutputMessage(item, ts, codexOutputText(ctx, item.Output))
	case "reasoning":
		if !reasoning {
			return msg, false
		}
		msg.Role, msg.Origin, msg.ContentType = "reasoning", models.OriginAssistant, "reasoning"
		msg.Content = sync.CleanContent(strings.Join([]string{
			codexTextBlocks(ctx, item.Summary, "summary_text"),
			codexTextBlocks(ctx, item.Content, "reasoning_text", "text"),
		}, " "))
	default:
		return msg, false
	}
	if ctx.Err() != nil {
		return msg, false
	}
	return msg, strings.TrimSpace(msg.Content) != ""
}

func codexOutputMessage(item codexItem, ts time.Time, text string) (models.Message, bool) {
	// Only explicit text is serialized; unknown objects remain metadata-only.
	raw, _ := json.Marshal(text)
	msg := models.Message{
		Timestamp: ts, Role: "tool", Origin: models.OriginAutomation,
		ContentType: "tool", Content: SerializeToolOutput(raw), ToolUseID: item.CallID,
	}
	return msg, strings.TrimSpace(msg.Content) != ""
}

func codexOutputText(ctx context.Context, raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		text = codexTextBlocks(ctx, raw, "input_text", "output_text")
	}
	return text
}

// Only native shell inputs prove a command. Custom inputs and generic exec
// wrappers contain free-form code, even when that code mentions shell commands.
func codexCommandHead(tool, arguments string) string {
	var command string
	switch tool {
	case "exec_command":
		var args struct {
			Cmd string `json:"cmd"`
		}
		if json.Unmarshal([]byte(arguments), &args) == nil {
			command = args.Cmd
		}
	case "shell":
		var args struct {
			Command []string `json:"command"`
		}
		if json.Unmarshal([]byte(arguments), &args) == nil && len(args.Command) > 0 {
			command = args.Command[0]
			if len(args.Command) == 3 && strings.HasSuffix(filepath.Base(command), "sh") && (args.Command[1] == "-c" || args.Command[1] == "-lc") {
				command = args.Command[2]
			}
		}
	}
	if command == "" {
		return ""
	}
	raw, _ := json.Marshal(struct {
		Command string `json:"command"`
	}{command})
	return commandHead(raw)
}

var codexProcessExit = regexp.MustCompile(`(?m)^Process exited with code (-?[0-9]+)\r?$`)

// Read native proof before toolfmt truncation. Shell JSON metadata and process
// envelopes describe that shell invocation; Script envelopes describe only the
// exec wrapper. Neither recursively interprets returned tool-response content.
func codexOutcome(tool string, item codexItem, text string) (*bool, *int) {
	isError := item.IsError
	var exitCode *int
	if tool == "exec_command" || tool == "shell" {
		raw := item.Output
		if len(raw) > 0 && raw[0] == '"' {
			raw = json.RawMessage(text)
		}
		var envelope struct {
			Metadata struct {
				ExitCode *int `json:"exit_code"`
			} `json:"metadata"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			exitCode = envelope.Metadata.ExitCode
		}
		if exitCode == nil {
			header, _, hasOutput := strings.Cut(text, "\nOutput:")
			if hasOutput && (strings.HasPrefix(header, "Chunk ID:") || strings.HasPrefix(header, "Wall time:") || strings.HasPrefix(header, "Process exited with code ")) {
				if match := codexProcessExit.FindStringSubmatch(header); match != nil {
					if code, err := strconv.Atoi(match[1]); err == nil {
						exitCode = new(code)
					}
				}
			}
		}
	} else if tool == "exec" && isError == nil {
		if strings.HasPrefix(text, "Script completed\n") || strings.HasPrefix(text, "Script failed\n") {
			isError = new(strings.HasPrefix(text, "Script failed\n"))
		}
	}
	if isError == nil && exitCode != nil {
		isError = new(*exitCode != 0)
	}
	return isError, exitCode
}

// codexTextBlocks selects only explicit text types, excluding arbitrary JSON
// and multimodal data. A malformed block does not hide valid sibling blocks.
func codexTextBlocks(ctx context.Context, raw json.RawMessage, types ...string) string {
	return strings.Join(codexTextParts(ctx, raw, types...), "\n")
}

func codexTextParts(ctx context.Context, raw json.RawMessage, types ...string) []string {
	var blocks []json.RawMessage
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	var parts []string
	for _, rawBlock := range blocks {
		if ctx.Err() != nil {
			return nil
		}
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
