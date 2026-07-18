package templates

import (
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
