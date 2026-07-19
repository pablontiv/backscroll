package templates

import (
	"strings"
	"testing"
)

func TestExtractBashErrors(t *testing.T) {
	output := `running test
FAIL: database timeout
exit status 1`
	lines := ExtractErrorLines("Bash", output)
	if len(lines) == 0 {
		t.Errorf("expected error lines, got none")
	}
	// Should contain both the FAIL line and last non-empty
	found := false
	for _, line := range lines {
		if line == "exit status 1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected last line 'exit status 1' in result: %v", lines)
	}
}

func TestExtractGoErrors(t *testing.T) {
	output := `./main.go:10: error: undefined identifier 'x'
FAIL github.com/user/pkg`
	lines := ExtractErrorLines("Go", output)
	if len(lines) < 2 {
		t.Errorf("expected at least 2 error lines, got %d: %v", len(lines), lines)
	}
}

func TestExtractGenericErrors(t *testing.T) {
	output := `setup
FATAL: something broke
cleanup`
	lines := ExtractErrorLines("Bash", output)
	if len(lines) == 0 {
		t.Errorf("expected error lines")
	}
}

func TestExtractEmptyInput(t *testing.T) {
	lines := ExtractErrorLines("Bash", "")
	if len(lines) != 0 {
		t.Errorf("expected empty result for empty input, got %v", lines)
	}
}

func TestExtractWhitespaceOnly(t *testing.T) {
	lines := ExtractErrorLines("Bash", "   \n\t\n  ")
	if len(lines) != 0 {
		t.Errorf("expected empty result for whitespace-only input, got %v", lines)
	}
}

func TestExtractUnknownTool(t *testing.T) {
	output := "error: something failed\nmore details"
	lines := ExtractErrorLines("UnknownTool", output)
	if len(lines) == 0 {
		t.Errorf("expected default extraction for unknown tool")
	}
}

func TestExtractCaseInsensitive(t *testing.T) {
	output := "output\nERROR: uppercase error\nmore"
	lines := ExtractErrorLines("Bash", output)
	if len(lines) == 0 {
		t.Errorf("expected case-insensitive error matching")
	}
}

func TestExtractMultipleErrors(t *testing.T) {
	output := `first line
ERROR: first error
middle line
FAIL: second error
last line`
	lines := ExtractErrorLines("Bash", output)
	if len(lines) < 2 {
		t.Errorf("expected multiple error lines, got %v", lines)
	}
}

// TestGoErrorsPatterns tests the new Go error extraction patterns:
// "--- FAIL:" for test headers, "FAIL\t" for summary lines, "error:" for compiler errors, panic().
func TestGoErrorsPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool // whether line should be extracted
	}{
		{
			name:     "test failure header",
			input:    "--- FAIL: TestFoo (1.23s)",
			expected: true,
		},
		{
			name:     "FAIL summary with tab",
			input:    "FAIL\tgithub.com/user/repo",
			expected: true,
		},
		{
			name:     "FAIL summary with spaces",
			input:    "FAIL    github.com/user/repo",
			expected: true,
		},
		{
			name:     "compiler error with colon",
			input:    "main.go:42: error: undefined variable",
			expected: true,
		},
		{
			name:     "panic invocation",
			input:    "panic(runtime error: slice bounds out of range)",
			expected: true,
		},
		{
			name:     "ok summary (no match)",
			input:    "ok\tgithub.com/user/repo",
			expected: false,
		},
		{
			name:     "normal output (no match)",
			input:    "Running TestFoo...",
			expected: false,
		},
		{
			name:     "colon error pattern",
			input:    "parse.go:10: error: bad syntax",
			expected: true, // matches ": error:" pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := extractGoErrors([]string{tt.input})
			got := len(lines) > 0
			if got != tt.expected {
				t.Errorf("extractGoErrors(%q) = %v, want %v; result: %v", tt.input, got, tt.expected, lines)
			}
		})
	}
}

// TestBashErrorsPatterns tests Bash error extraction:
// last non-empty line + any line matching error/fail/panic/denied/fatal (case-insensitive).
func TestBashErrorsPatterns(t *testing.T) {
	tests := []struct {
		name          string
		input         []string
		expectCount   int    // minimum expected results
		expectLastNEL bool   // whether last non-empty line should be included
		expectContent string // substring that should appear in result
	}{
		{
			name:          "simple error output",
			input:         []string{"building...", "error: undefined symbol", "cleanup"},
			expectCount:   2,    // error line + last non-empty
			expectLastNEL: true, // "cleanup" should be included
			expectContent: "error",
		},
		{
			name:          "exit code only",
			input:         []string{"output", "exit code: 1"},
			expectCount:   1,
			expectLastNEL: true,
			expectContent: "exit",
		},
		{
			name:          "panic in output",
			input:         []string{"running...", "panic: out of memory"},
			expectCount:   1,
			expectLastNEL: false, // last non-empty is already the panic line
			expectContent: "panic",
		},
		{
			name:          "multiple errors",
			input:         []string{"start", "error: first", "middle", "fail: second", "end"},
			expectCount:   3, // two error lines + last non-empty
			expectLastNEL: true,
			expectContent: "error",
		},
		{
			name:          "case insensitive",
			input:         []string{"output", "ERROR: uppercase", "FAIL: test"},
			expectCount:   2,
			expectLastNEL: false, // FAIL is already captured
			expectContent: "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBashErrors(tt.input)
			if len(result) < tt.expectCount {
				t.Errorf("%s: expected >= %d results, got %d: %v", tt.name, tt.expectCount, len(result), result)
			}
			if tt.expectContent != "" {
				found := false
				for _, line := range result {
					if strings.Contains(strings.ToLower(line), strings.ToLower(tt.expectContent)) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: expected content %q in results, got %v", tt.name, tt.expectContent, result)
				}
			}
		})
	}
}
