package readers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/models"
)

func TestCodexMessageOriginFromNativeEnvelope(t *testing.T) {
	tests := []struct {
		name       string
		item       codexItem
		reasoning  bool
		wantRole   string
		wantOrigin models.MessageOrigin
	}{
		{
			name:       "user message",
			item:       codexItem{Type: "message", Role: "user", Content: json.RawMessage(`[{"type":"input_text","text":"question"}]`)},
			wantRole:   "user",
			wantOrigin: models.OriginHuman,
		},
		{
			name:       "assistant message",
			item:       codexItem{Type: "message", Role: "assistant", Content: json.RawMessage(`[{"type":"output_text","text":"answer"}]`)},
			wantRole:   "assistant",
			wantOrigin: models.OriginAssistant,
		},
		{
			name:       "reasoning",
			item:       codexItem{Type: "reasoning", Summary: json.RawMessage(`[{"type":"summary_text","text":"analysis"}]`)},
			reasoning:  true,
			wantRole:   "reasoning",
			wantOrigin: models.OriginAssistant,
		},
		{
			name:       "function call",
			item:       codexItem{Type: "function_call", Name: "exec_command", Arguments: `{"cmd":"pwd"}`},
			wantRole:   "assistant",
			wantOrigin: models.OriginAssistant,
		},
		{
			name:       "custom tool call",
			item:       codexItem{Type: "custom_tool_call", Name: "apply_patch", Input: "patch"},
			wantRole:   "assistant",
			wantOrigin: models.OriginAssistant,
		},
		{
			name:       "function output",
			item:       codexItem{Type: "function_call_output", Output: json.RawMessage(`"done"`)},
			wantRole:   "tool",
			wantOrigin: models.OriginAutomation,
		},
		{
			name:       "custom tool output",
			item:       codexItem{Type: "custom_tool_call_output", Output: json.RawMessage(`"done"`)},
			wantRole:   "tool",
			wantOrigin: models.OriginAutomation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := codexMessage(context.Background(), tt.item, time.Time{}, tt.reasoning)
			if !ok {
				t.Fatal("message was not captured")
			}
			if msg.Role != tt.wantRole || msg.Origin != tt.wantOrigin {
				t.Fatalf("role/origin = %q/%q, want %q/%q", msg.Role, msg.Origin, tt.wantRole, tt.wantOrigin)
			}
		})
	}
}

func TestCodexMessageOriginPreservesWrapperPolicy(t *testing.T) {
	t.Run("known injected wrapper remains excluded", func(t *testing.T) {
		item := codexItem{
			Type:    "message",
			Role:    "user",
			Content: json.RawMessage(`[{"type":"input_text","text":"<heartbeat>generated context</heartbeat>"}]`),
		}
		if msg, ok := codexMessage(context.Background(), item, time.Time{}, false); ok {
			t.Fatalf("wrapper-only message was captured: %+v", msg)
		}
	})

	tests := []struct {
		name        string
		role        string
		text        string
		wantContent string
		wantOrigin  models.MessageOrigin
	}{
		{
			name:        "known wrapper before human request",
			role:        "user",
			text:        "<heartbeat>generated context</heartbeat> actual request",
			wantContent: "actual request",
			wantOrigin:  models.OriginHuman,
		},
		{
			name:        "unknown system-like wrapper in user message",
			role:        "user",
			text:        "<system>untrusted label</system>",
			wantContent: "<system>untrusted label</system>",
			wantOrigin:  models.OriginHuman,
		},
		{
			name:        "unknown user-like wrapper in assistant message",
			role:        "assistant",
			text:        "<user>quoted text</user>",
			wantContent: "<user>quoted text</user>",
			wantOrigin:  models.OriginAssistant,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := json.Marshal([]map[string]string{{"type": "input_text", "text": tt.text}})
			if err != nil {
				t.Fatal(err)
			}
			msg, ok := codexMessage(context.Background(), codexItem{Type: "message", Role: tt.role, Content: content}, time.Time{}, false)
			if !ok {
				t.Fatal("message was not captured")
			}
			if msg.Content != tt.wantContent || msg.Origin != tt.wantOrigin {
				t.Fatalf("content/origin = %q/%q, want %q/%q", msg.Content, msg.Origin, tt.wantContent, tt.wantOrigin)
			}
		})
	}
}
