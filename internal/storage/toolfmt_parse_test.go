package storage

import (
	"testing"
)

func TestParseToolFromSerializedExtractsInputToolName(t *testing.T) {
	// Input format: "Bash command=go test ..."
	toolName, cmdHead := ParseToolFromSerialized("Bash command=go test ./...")
	if toolName != "Bash" {
		t.Errorf("tool_name = %q, want Bash", toolName)
	}
	if cmdHead != "go" {
		t.Errorf("command_head = %q, want go", cmdHead)
	}
}

func TestParseToolFromSerializedExtractsCommandHead(t *testing.T) {
	toolName, cmdHead := ParseToolFromSerialized("Bash command=npm run build --prod")
	if toolName != "Bash" {
		t.Errorf("tool_name = %q, want Bash", toolName)
	}
	if cmdHead != "npm" {
		t.Errorf("command_head = %q, want npm", cmdHead)
	}
}

func TestParseToolFromSerializedHandlesNonCLITools(t *testing.T) {
	// Read tool with file_path= (not command=)
	toolName, cmdHead := ParseToolFromSerialized("Read file_path=/x.go")
	if toolName != "Read" {
		t.Errorf("tool_name = %q, want Read", toolName)
	}
	if cmdHead != "" {
		t.Errorf("command_head = %q, want empty (no command=)", cmdHead)
	}

	// Edit tool with multiple keys
	toolName, cmdHead = ParseToolFromSerialized("Edit file_path=/a old_string=b new_string=c")
	if toolName != "Edit" {
		t.Errorf("tool_name = %q, want Edit", toolName)
	}
	if cmdHead != "" {
		t.Errorf("command_head = %q, want empty", cmdHead)
	}
}

func TestParseToolFromSerializedSkipsOutputs(t *testing.T) {
	// Free-form output text with no key=value pattern
	toolName, _ := ParseToolFromSerialized("PASS: all tests passed")
	if toolName != "" {
		t.Errorf("output should yield empty toolName; got %q", toolName)
	}

	// "error: ..." output (no key=value pair) → skip
	toolName, _ = ParseToolFromSerialized("error: exit code 1")
	if toolName != "" {
		t.Errorf("error output should yield empty toolName; got %q", toolName)
	}

	// Output that happens to contain "=" but doesn't match input structure
	// (e.g., "result_1_field=value message_id=1" without a leading tool name)
	toolName, _ = ParseToolFromSerialized("result_1_field=value message_id=123")
	if toolName != "" {
		t.Errorf("garbage output should yield empty toolName; got %q", toolName)
	}
}

func TestParseToolFromSerializedEdgeCases(t *testing.T) {
	// Empty string
	toolName, cmdHead := ParseToolFromSerialized("")
	if toolName != "" || cmdHead != "" {
		t.Errorf("empty string should yield empty results; got %q, %q", toolName, cmdHead)
	}

	// Whitespace only
	toolName, cmdHead = ParseToolFromSerialized("   ")
	if toolName != "" || cmdHead != "" {
		t.Errorf("whitespace-only should yield empty results; got %q, %q", toolName, cmdHead)
	}

	// Single token (no key=value)
	toolName, cmdHead = ParseToolFromSerialized("error")
	if toolName != "" || cmdHead != "" {
		t.Errorf("single token should yield empty results; got %q, %q", toolName, cmdHead)
	}

	// Tool with multiple command-like values
	toolName, cmdHead = ParseToolFromSerialized("Bash command=find /path -name '*.go'")
	if toolName != "Bash" {
		t.Errorf("tool_name = %q, want Bash", toolName)
	}
	if cmdHead != "find" {
		t.Errorf("command_head = %q, want find", cmdHead)
	}
}
