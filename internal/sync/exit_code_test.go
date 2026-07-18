package sync

import (
	"testing"
)

func TestExtractExitCode(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		toolName string
		want     *int
	}{
		{
			name:     "bash with exit code pattern 1",
			text:     "FAIL: some error\nexit code 1",
			toolName: "Bash",
			want:     ptrInt(1),
		},
		{
			name:     "bash with exit code pattern 2",
			text:     "Build failed: Exit code: 127",
			toolName: "Bash",
			want:     ptrInt(127),
		},
		{
			name:     "bash zero exit code",
			text:     "all tests passed\nexit code 0",
			toolName: "Bash",
			want:     ptrInt(0),
		},
		{
			name:     "bash no pattern",
			text:     "some output without exit code",
			toolName: "Bash",
			want:     nil,
		},
		{
			name:     "non-bash tool",
			text:     "exit code 5",
			toolName: "Read",
			want:     nil,
		},
		{
			name:     "empty text",
			text:     "",
			toolName: "Bash",
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractExitCode(tt.text, tt.toolName)
			if (got == nil && tt.want != nil) || (got != nil && tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("ExtractExitCode(%q, %q) = %v, want %v", tt.text, tt.toolName, got, tt.want)
			}
		})
	}
}

func ptrInt(i int) *int {
	return &i
}
