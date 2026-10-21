package sync

import "testing"

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
			name:  "multiline fenced whitespace collapse",
			input: "  before\n```go\nvalue :=  167\n```\n after\tmarker\u00a0 ",
			want:  "before ```go value := 167 ``` after marker",
		},
		{
			name:  "provider removal before whitespace",
			input: "Caveat: before <system-reminder>```hidden```</system-reminder>\n\t after",
			want:  "before after",
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
