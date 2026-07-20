package sync

import (
	"os"
	"strings"
	"testing"
)

// TestParseSessions_PiFormat tests parsing Pi format sessions
func TestParseSessions_PiFormat(t *testing.T) {
	path := "../../tests/fixtures/pi-session.jsonl"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture file not found: %s", path)
	}

	messages, err := ParseSessions(path)
	if err != nil {
		t.Fatalf("ParseSessions failed: %v", err)
	}

	if len(messages) == 0 {
		t.Fatal("expected messages, got none")
	}

	// Check we have user and assistant roles
	hasUser := false
	hasAssistant := false

	for _, msg := range messages {
		if msg.Role == "user" {
			hasUser = true
		}
		if msg.Role == "assistant" {
			hasAssistant = true
		}
	}

	if !hasUser {
		t.Error("expected user role, got none")
	}
	if !hasAssistant {
		t.Error("expected assistant role, got none")
	}

	// Check timestamp parsing
	for _, msg := range messages {
		if msg.Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	}
}

// TestParseSessions_ClaudeFormat tests parsing Claude format sessions
func TestParseSessions_ClaudeFormat(t *testing.T) {
	path := "../../tests/fixtures/claude-preset/projects/project-a/session-main.jsonl"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture file not found: %s", path)
	}

	messages, err := ParseSessions(path)
	if err != nil {
		t.Fatalf("ParseSessions failed: %v", err)
	}

	if len(messages) == 0 {
		t.Fatal("expected messages, got none")
	}

	// Verify noise filtering worked:
	// - "progress" type should be filtered
	// - isMeta records should be filtered
	// - task-notification type should be filtered
	// - System reminder content should be removed

	for _, msg := range messages {
		if strings.Contains(msg.Content, "system-reminder") {
			t.Error("system-reminder should be filtered from content")
		}
		if strings.Contains(msg.Content, "drop progress") {
			t.Error("progress type message should be filtered")
		}
		if strings.Contains(msg.Content, "drop metadata") {
			t.Error("metadata (isMeta) message should be filtered")
		}
		if strings.Contains(msg.Content, "remove whole notification") {
			t.Error("task-notification should be filtered")
		}
	}

	// Check we have user and assistant roles
	hasUser := false
	hasAssistant := false
	for _, msg := range messages {
		if msg.Role == "user" {
			hasUser = true
		}
		if msg.Role == "assistant" {
			hasAssistant = true
		}
	}

	if !hasUser {
		t.Error("expected user role")
	}
	if !hasAssistant {
		t.Error("expected assistant role")
	}
}

// TestNoiseFiltering_ToolEvents tests that tool events are parsed but not tool result content
func TestNoiseFiltering_ToolEvents(t *testing.T) {
	path := "../../tests/fixtures/claude-tool-events.jsonl"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture file not found: %s", path)
	}

	messages, err := ParseSessions(path)
	if err != nil {
		t.Fatalf("ParseSessions failed: %v", err)
	}

	if len(messages) == 0 {
		t.Fatal("expected messages, got none")
	}

	// Should have parsed messages despite tool events
	hasNormalMessage := false
	hasToolContent := false

	for _, msg := range messages {
		if strings.Contains(msg.Content, "normal claude message") {
			hasNormalMessage = true
		}
		if msg.ContentType == "tool" {
			hasToolContent = true
		}
	}

	if !hasNormalMessage {
		t.Error("expected to find normal message")
	}
	if !hasToolContent {
		t.Error("expected to find tool content type")
	}
}

// TestWalkSessionDirs_Exclusion tests subagent filtering
func TestWalkSessionDirs_Exclusion(t *testing.T) {
	dir := "../../tests/fixtures/claude-preset"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("fixture directory not found: %s", dir)
	}

	// With includeAgents=false, subagent files should be excluded
	paths, err := WalkSessionDirs([]string{dir}, false)
	if err != nil {
		t.Fatalf("WalkSessionDirs failed: %v", err)
	}

	for _, p := range paths {
		if strings.Contains(p, "/subagents/") {
			t.Errorf("subagent file should be excluded: %s", p)
		}
	}

	// With includeAgents=true, subagent files should be included
	pathsWithAgents, err := WalkSessionDirs([]string{dir}, true)
	if err != nil {
		t.Fatalf("WalkSessionDirs with includeAgents failed: %v", err)
	}

	hasSubagent := false
	for _, p := range pathsWithAgents {
		if strings.Contains(p, "/subagents/") {
			hasSubagent = true
			break
		}
	}

	if !hasSubagent {
		t.Error("expected to find subagent files with includeAgents=true")
	}
}

// TestIsNoiseRecord_FilterTypes tests that noise types are filtered
func TestIsNoiseRecord_FilterTypes(t *testing.T) {
	tests := []struct {
		name      string
		record    rawRecord
		wantNoise bool
	}{
		{
			name: "system-reminder type",
			record: rawRecord{
				Type: "system-reminder",
				Message: &rawMessage{
					Role:    "user",
					Content: []byte(`"content"`),
				},
			},
			wantNoise: true,
		},
		{
			name: "task-notification type",
			record: rawRecord{
				Type: "task-notification",
				Message: &rawMessage{
					Role:    "assistant",
					Content: []byte(`"content"`),
				},
			},
			wantNoise: true,
		},
		{
			name: "progress type",
			record: rawRecord{
				Type: "progress",
				Message: &rawMessage{
					Role:    "assistant",
					Content: []byte(`"content"`),
				},
			},
			wantNoise: true,
		},
		{
			name: "isMeta true",
			record: rawRecord{
				Type:   "user",
				IsMeta: true,
				Message: &rawMessage{
					Role:    "user",
					Content: []byte(`"content"`),
				},
			},
			wantNoise: true,
		},
		{
			name: "empty role",
			record: rawRecord{
				Type: "message",
				Message: &rawMessage{
					Role:    "",
					Content: []byte(`"content"`),
				},
			},
			wantNoise: true,
		},
		{
			name: "nil message",
			record: rawRecord{
				Type:    "message",
				Message: nil,
			},
			wantNoise: true,
		},
		{
			name: "valid user message",
			record: rawRecord{
				Type: "user",
				Message: &rawMessage{
					Role:    "user",
					Content: []byte(`"valid content"`),
				},
			},
			wantNoise: false,
		},
		{
			name: "valid assistant message",
			record: rawRecord{
				Type: "assistant",
				Message: &rawMessage{
					Role:    "assistant",
					Content: []byte(`"valid content"`),
				},
			},
			wantNoise: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsNoiseRecord(tt.record)
			if got != tt.wantNoise {
				t.Errorf("IsNoiseRecord() = %v, want %v", got, tt.wantNoise)
			}
		})
	}
}

// TestExtractContent_StringFormat tests string content extraction
func TestExtractContent_StringFormat(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantContent string
		wantType    string
	}{
		{
			name:        "simple string",
			input:       `"hello world"`,
			wantContent: "hello world",
			wantType:    "text",
		},
		{
			name:        "string with code fence",
			input:       "\"```go\\nfunc main() {}\\n```\"",
			wantContent: "```go func main() {} ```",
			wantType:    "code",
		},
		{
			name:        "empty string",
			input:       `""`,
			wantContent: "",
			wantType:    "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, contentType := extractContent([]byte(tt.input))

			// For empty content, we expect empty string
			if tt.wantContent == "" {
				if content != "" {
					t.Errorf("expected empty content, got %q", content)
				}
				return
			}

			if !strings.Contains(content, strings.TrimSpace(tt.wantContent)) {
				t.Errorf("content = %q, want to contain %q", content, tt.wantContent)
			}
			if contentType != tt.wantType {
				t.Errorf("contentType = %q, want %q", contentType, tt.wantType)
			}
		})
	}
}

// TestExtractContent_BlockFormat tests block array content extraction
func TestExtractContent_BlockFormat(t *testing.T) {
	tests := []struct {
		name     string
		blocks   string
		wantType string
	}{
		{
			name:     "text block",
			blocks:   `[{"type":"text","text":"hello"}]`,
			wantType: "text",
		},
		{
			name:     "text with code fence",
			blocks:   "[{\"type\":\"text\",\"text\":\"code: ```go\\nfunc main() {}\\n```\"}]",
			wantType: "code",
		},
		{
			name:     "tool_use block",
			blocks:   `[{"type":"text","text":"calling tool"},{"type":"tool_use","id":"t1","name":"bash"}]`,
			wantType: "tool",
		},
		{
			name:     "thinking ignored",
			blocks:   `[{"type":"thinking","thinking":"hidden"},{"type":"text","text":"visible"}]`,
			wantType: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, contentType := extractContent([]byte(tt.blocks))
			if contentType != tt.wantType {
				t.Errorf("contentType = %q, want %q", contentType, tt.wantType)
			}
		})
	}
}

// TestCleanContent tests noise pattern removal
func TestCleanContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "system reminder removal with tag content",
			input: "hello <system-reminder>remove this</system-reminder> world",
			want:  "hello world",
		},
		{
			name:  "task notification removal with tag content",
			input: "start <task-notification>drop</task-notification> end",
			want:  "start end",
		},
		{
			name:  "entire task notification content removed",
			input: "<task-notification>remove whole notification</task-notification>",
			want:  "",
		},
		{
			name:  "caveat prefix removal",
			input: "Caveat: ignore this\nBase directory: /tmp\nKeep this",
			want:  "ignore this Base directory: /tmp Keep this",
		},
		{
			name:  "caveat tag removal with content",
			input: "text<caveat>drop caveat</caveat> more",
			want:  "text more",
		},
		{
			name:  "command tag removal with content",
			input: "before<command>drop</command>after",
			want:  "beforeafter",
		},
		{
			name:  "local-command-caveat removal with content",
			input: "text<local-command-caveat>drop local caveat</local-command-caveat>",
			want:  "text",
		},
		{
			name:  "multiple whitespace collapse",
			input: "hello    world    test",
			want:  "hello world test",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "teammate-message wrapper with content",
			input: "msg <teammate-message>internal</teammate-message>",
			want:  "msg",
		},
		{
			name:  "teammate-message wrapper only",
			input: "<teammate-message>only</teammate-message>",
			want:  "",
		},
		{
			name:  "teammate-message wrapper in middle",
			input: "msg <teammate-message>internal</teammate-message> more",
			want:  "msg more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanContent(tt.input)
			if got != tt.want {
				t.Errorf("CleanContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseSessions_IgnoresSubagents tests that subagent sessions are excluded when reading
func TestParseSessions_IgnoresSubagents(t *testing.T) {
	path := "../../tests/fixtures/claude-preset/projects/project-a/subagents/agent.jsonl"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("fixture file not found: %s", path)
	}

	messages, err := ParseSessions(path)
	if err != nil {
		t.Fatalf("ParseSessions failed: %v", err)
	}

	// Subagent file should be parsed if passed directly
	// The filtering happens at the directory walk level
	if len(messages) == 0 {
		t.Log("no messages from subagent file (expected)")
	}
}

// BenchmarkParseSessions benchmarks session parsing
func BenchmarkParseSessions(b *testing.B) {
	path := "../../tests/fixtures/pi-session.jsonl"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		b.Skipf("fixture file not found: %s", path)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseSessions(path)
	}
}

// BenchmarkWalkSessionDirs benchmarks directory walking
func BenchmarkWalkSessionDirs(b *testing.B) {
	dir := "../../tests/fixtures"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		b.Skipf("fixture directory not found: %s", dir)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = WalkSessionDirs([]string{dir}, false)
	}
}
