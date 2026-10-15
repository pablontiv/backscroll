package readers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pablontiv/backscroll/internal/input_config"
	"github.com/pablontiv/backscroll/internal/models"
	"github.com/pablontiv/backscroll/internal/sync"
)

// OMPReader reads native filesystem OMP v3 JSONL, not Pi or generic JSONL.
// Only explicit message content is indexed; see docs/input-contract.md.
type OMPReader struct{}

func (*OMPReader) Name() string           { return "omp" }
func (*OMPReader) IngestionEpoch() uint64 { return 1 }
func (*OMPReader) Discover(ctx context.Context, def input_config.InputDefinition) ([]string, error) {
	return input_config.DiscoverFilesContext(ctx, def.Discover)
}
func (*OMPReader) Hash(ctx context.Context, path string) (string, error) { return hashFile(ctx, path) }

type ompRecord struct {
	Type      string      `json:"type"`
	Version   int         `json:"version"`
	ID        string      `json:"id"`
	CWD       string      `json:"cwd"`
	Timestamp string      `json:"timestamp"`
	Message   *ompMessage `json:"message"`
}

type ompMessage struct {
	Role        string          `json:"role"`
	Content     json.RawMessage `json:"content"`
	Attribution string          `json:"attribution"`
	Synthetic   bool            `json:"synthetic"`
	ToolCallID  string          `json:"toolCallId"`
	ToolName    string          `json:"toolName"`
	IsError     *bool           `json:"isError"`
	Details     json.RawMessage `json:"details"`
}

type ompBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ompResultSignal struct {
	callID      string
	toolName    string
	isError     *bool
	exitCode    *int
	interrupted bool
	indices     []int
}

// Parse preserves physical message/block order, including stored branch history.
// parentId is not a tool linkage. Malformed lines are skipped, but a missing,
// duplicate or unsupported header rejects the file rather than inventing IDs.
func (*OMPReader) Parse(ctx context.Context, path string, def input_config.InputDefinition) (models.ParsedFile, error) {
	hash, err := hashFile(ctx, path)
	if err != nil {
		return models.ParsedFile{}, err
	}
	parsed := models.ParsedFile{Path: path, Hash: hash}
	var sessionID string
	calls := map[string]int{}
	var results []ompResultSignal
	err = sync.IterateJSONLFileContext(ctx, path, func(_ int, line []byte) error {
		var rec ompRecord
		if json.Unmarshal(line, &rec) != nil {
			return nil
		}
		if rec.Type == "session" {
			_, timestampErr := time.Parse(time.RFC3339Nano, rec.Timestamp)
			if sessionID != "" || rec.Version != 3 || strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.CWD) == "" || timestampErr != nil {
				return fmt.Errorf("omp: expected one native v3 session header")
			}
			sessionID, parsed.Cwd = rec.ID, rec.CWD
			return nil
		}
		if rec.Type != "message" || sessionID == "" || rec.ID == "" || rec.Message == nil {
			return nil
		}
		ts, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
		if err != nil {
			return nil
		}
		m := rec.Message
		origin := ompOrigin(m)
		if m.Role != "user" && m.Role != "assistant" && m.Role != "developer" && m.Role != "toolResult" {
			return nil
		}
		base := models.Message{Role: m.Role, Origin: origin, Timestamp: ts}
		emit := func(index int, text, kind string) int {
			msg := base
			msg.UUID = fmt.Sprintf("omp:%s:%s#b%d", sessionID, rec.ID, index)
			msg.Content, msg.ContentType = text, kind
			parsed.Records = append(parsed.Records, msg)
			return len(parsed.Records) - 1
		}
		var text string
		if m.Role != "toolResult" && json.Unmarshal(m.Content, &text) == nil {
			if cleaned := sync.CleanContent(text); cleaned != "" {
				emit(0, cleaned, classifyText(cleaned))
			}
			return nil
		}
		var blocks []ompBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			return nil
		}
		if m.Role == "toolResult" {
			// Read only native metadata fields, independently: an unsupported details
			// value must not erase an otherwise valid isError/content boundary.
			signal := ompResultSignal{callID: m.ToolCallID, toolName: m.ToolName, isError: m.IsError}
			var details map[string]json.RawMessage
			if json.Unmarshal(m.Details, &details) == nil {
				_ = json.Unmarshal(details["__interrupted"], &signal.interrupted)
				if m.ToolName == "bash" && string(details["exitCode"]) != "null" {
					var code int
					if json.Unmarshal(details["exitCode"], &code) == nil {
						signal.exitCode = &code
					}
				}
			}
			for i, b := range blocks {
				if err := ctx.Err(); err != nil {
					return err
				}
				if b.Type != "text" || strings.TrimSpace(b.Text) == "" {
					continue
				}
				base.Role = "tool"
				idx := emit(i, truncateRunes(b.Text, MaxToolTextLen), "tool")
				parsed.Records[idx].ToolUseID = m.ToolCallID
				parsed.Records[idx].IsError = signal.isError
				parsed.Records[idx].ExitCode = signal.exitCode
				parsed.Records[idx].WasInterrupted = signal.interrupted
				signal.indices = append(signal.indices, idx)
			}
			results = append(results, signal)
			return nil
		}
		for i, b := range blocks {
			if err := ctx.Err(); err != nil {
				return err
			}
			switch b.Type {
			case "text":
				if cleaned := sync.CleanContent(b.Text); cleaned != "" {
					emit(i, cleaned, classifyText(cleaned))
				}
			case "thinking":
				if m.Role == "assistant" && def.Decode.IndexReasoning {
					if cleaned := sync.CleanContent(b.Thinking); cleaned != "" {
						base.Role = "reasoning"
						emit(i, cleaned, "reasoning")
						base.Role = m.Role
					}
				}
			case "toolCall":
				var args map[string]json.RawMessage
				if m.Role != "assistant" || b.ID == "" || b.Name == "" || json.Unmarshal(b.Arguments, &args) != nil || args == nil {
					continue
				}
				idx := emit(i, SerializeToolInput(b.Name, b.Arguments), "tool")
				msg := &parsed.Records[idx]
				msg.ToolName, msg.ToolUseID = b.Name, b.ID
				msg.CommandHead = commandHead(b.Arguments)
				msg.SearchEcho = isDirectSearchInput(b.Name, b.Arguments)
				calls[b.ID] = idx
			}
		}
		return nil
	})
	if err != nil {
		return models.ParsedFile{}, err
	}
	if sessionID == "" {
		return models.ParsedFile{}, fmt.Errorf("omp: missing native v3 session header")
	}
	// Pair from raw IDs after reading the file, including out-of-order results.
	for _, result := range results {
		if err := ctx.Err(); err != nil {
			return models.ParsedFile{}, err
		}
		idx, ok := calls[result.callID]
		if !ok || result.callID == "" || parsed.Records[idx].ToolName != result.toolName {
			continue
		}
		call := &parsed.Records[idx]
		call.IsError, call.WasInterrupted, call.ExitCode = result.isError, result.interrupted, result.exitCode
		for _, i := range result.indices {
			parsed.Records[i].SearchEcho = call.SearchEcho
		}
	}
	return parsed, nil
}

func ompOrigin(m *ompMessage) models.MessageOrigin {
	switch m.Role {
	case "assistant":
		return models.OriginAssistant
	case "toolResult":
		return models.OriginAutomation
	case "developer":
		return models.OriginSystem
	case "user":
		if m.Synthetic || m.Attribution == "agent" {
			return models.OriginAutomation
		}
		if m.Attribution == "user" {
			return models.OriginHuman
		}
	}
	return models.OriginUnknown
}
