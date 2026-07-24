package storage

import (
	"testing"
)

func TestIsInputSerialization(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Input serializations (should return true — skip from mining)
		{"Bash command=cd /foo && npm run test", true},
		{"Go command=go test ./...", true},
		{"TaskOutput block=false timeout=5000", true},
		{"Git command=git log --oneline", true},
		// Error output (should return false — include in mining)
		{"error: connection refused", false},
		{"FAIL: test suite", false},
		{"undefined variable x", false},
		{"", false}, // Empty string
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isInputSerialization(tt.input)
			if got != tt.expected {
				t.Errorf("isInputSerialization(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
