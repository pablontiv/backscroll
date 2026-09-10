package readers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestStripCodexInjectedWrappers(t *testing.T) {
	for _, tag := range []string{"recommended_plugins", "environment_context", "heartbeat", "turn_aborted"} {
		t.Run(tag, func(t *testing.T) {
			wrapper := "<" + tag + ">" + strings.Repeat("synthetic boilerplate ", 3000) + "</" + tag + ">"
			if got := stripCodexInjectedWrappers(" \n" + wrapper + "\t "); got != "" {
				t.Fatalf("complete wrapper retained %d bytes", len(got))
			}
			if got := stripCodexInjectedWrappers(wrapper + "\n actual request"); got != "actual request" {
				t.Fatalf("lost trailing request: %q", got)
			}
		})
	}
	for _, tc := range []struct{ name, text, want string }{
		{"empty", "  ", ""},
		{"ordinary", "  actual user request  ", "actual user request"},
		{"task", "<task>assignment</task>", "<task>assignment</task>"},
		{"unknown", "<user_instructions>assignment</user_instructions>", "<user_instructions>assignment</user_instructions>"},
		{"embedded", "Explain <heartbeat>sample</heartbeat>", "Explain <heartbeat>sample</heartbeat>"},
		{"fenced", "```xml\n<heartbeat>sample</heartbeat>\n```", "```xml\n<heartbeat>sample</heartbeat>\n```"},
		{"incomplete", "<heartbeat>keep all remaining text", "<heartbeat>keep all remaining text"},
		{"mismatched", "<heartbeat>text</task>", "<heartbeat>text</task>"},
		{"attributes", `<heartbeat origin="user">text</heartbeat>`, `<heartbeat origin="user">text</heartbeat>`},
		{"case sensitive", "<Heartbeat>text</Heartbeat>", "<Heartbeat>text</Heartbeat>"},
		{"consecutive", "<heartbeat>one</heartbeat>\n<environment_context>two</environment_context>\n<task>keep</task>", "<task>keep</task>"},
		{"empty pair", "<turn_aborted></turn_aborted> request", "request"},
		{"closing only", "</heartbeat> keep", "</heartbeat> keep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripCodexInjectedWrappers(tc.text); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestCodexWrapperPolicyDoesNotTouchToolOrReasoningText(t *testing.T) {
	const text = "<heartbeat>searchable evidence</heartbeat>"
	raw, _ := json.Marshal(text)
	for _, item := range []codexItem{
		{Type: "function_call_output", Output: raw},
		{Type: "custom_tool_call_output", Output: raw},
		{Type: "reasoning", Summary: json.RawMessage(`[{"type":"summary_text","text":"<heartbeat>searchable evidence</heartbeat>"}]`)},
	} {
		msg, ok := codexMessage(item, time.Time{}, true)
		if !ok || msg.Content != text {
			t.Fatalf("policy leaked into %s: %+v", item.Type, msg)
		}
	}
}

func FuzzStripCodexInjectedWrappers(f *testing.F) {
	f.Add("<recommended_plugins>catalog</recommended_plugins> actual request")
	f.Add("<heartbeat>one</heartbeat><environment_context>two</environment_context>")
	f.Add("<heartbeat>unclosed")
	f.Fuzz(func(t *testing.T, text string) {
		got := stripCodexInjectedWrappers(text)
		if !strings.HasSuffix(strings.TrimSpace(text), got) {
			t.Fatal("exclusion changed non-prefix content")
		}
		if stripCodexInjectedWrappers(got) != got {
			t.Fatal("exclusion was not idempotent")
		}
	})
}
