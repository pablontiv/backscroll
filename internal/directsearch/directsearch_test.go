package directsearch

import "testing"

func TestIsDirectSearchCommand(t *testing.T) {
	tests := []struct {
		name, command string
		want          bool
	}{
		{"bare", "backscroll search", true},
		{"with_flags", "backscroll search --text orchard", true},
		{"only_two_tokens", "backscroll search", true},
		{"missing_search", "backscroll", false},
		{"missing_backscroll", "search", false},
		{"empty", "", false},
		{"whitespace_separated", "backscroll\tsearch", true},
		{"newline_separated", "backscroll\nsearch", true},
		{"nbsp_separated", "backscroll\u00a0search", true},
		{"em_space_separated", "backscroll\u2003search", true},
		{"wrapped_in_another", "env backscroll search", false}, // Fields[0]="env" not "backscroll"
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDirectSearchCommand(tt.command); got != tt.want {
				t.Errorf("IsDirectSearchCommand(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestIsCodexDirectSearchCall_Shell(t *testing.T) {
	tests := []struct {
		name, tool, arguments string
		want                  bool
	}{
		{
			name:      "bare_three_element_c",
			tool:      "shell",
			arguments: `{"command":["sh","-c","backscroll search"]}`,
			want:      true,
		},
		{
			name:      "three_element_with_extra_flags",
			tool:      "shell",
			arguments: `{"command":["sh","-c","backscroll search --text orchard"]}`,
			want:      true,
		},
		{
			name:      "three_element_lc",
			tool:      "shell",
			arguments: `{"command":["bash","-lc","backscroll search --text orchard"]}`,
			want:      true,
		},
		{
			name:      "shell_binary_path",
			tool:      "shell",
			arguments: `{"command":["/bin/bash","-c","backscroll search"]}`,
			want:      true,
		},
		{
			name:      "four_elements_rejected",
			tool:      "shell",
			arguments: `{"command":["sh","-c","backscroll search","extra"]}`,
			want:      false,
		},
		{
			name:      "wrong_flag",
			tool:      "shell",
			arguments: `{"command":["sh","-x","backscroll search"]}`,
			want:      false,
		},
		{
			name:      "different_command",
			tool:      "shell",
			arguments: `{"command":["sh","-c","ls"]}`,
			want:      false,
		},
		{
			name:      "not_a_shell_binary",
			tool:      "shell",
			arguments: `{"command":["python","-c","backscroll search"]}`,
			want:      false,
		},
		{
			name:      "arguments_extra_fields_decoded",
			tool:      "shell",
			arguments: `{"additional_permissions":{"network":false},"command":["bash","-lc","backscroll search"]}`,
			want:      true,
		},
		{
			name:      "arguments_extra_fields_in_storage_position",
			tool:      "shell",
			arguments: `{"command":["sh","-c","backscroll search"],"workdir":"/tmp","timeout_ms":10000}`,
			want:      true,
		},
		{
			name:      "malformed_arguments",
			tool:      "shell",
			arguments: `not json`,
			want:      false,
		},
		{
			name:      "command_separator_tab_accepted",
			tool:      "shell",
			arguments: `{"command":["sh","-c","backscroll\tsearch"]}`,
			want:      true,
		},
		{
			name:      "command_separator_nbsp_accepted",
			tool:      "shell",
			arguments: `{"command":["sh","-c","backscroll\u00a0search"]}`,
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCodexDirectSearchCall(tt.tool, tt.arguments); got != tt.want {
				t.Errorf("IsCodexDirectSearchCall(%q, %q) = %v, want %v", tt.tool, tt.arguments, got, tt.want)
			}
		})
	}
}

func TestIsCodexDirectSearchCall_ExecCommand(t *testing.T) {
	tests := []struct {
		name, tool, arguments string
		want                  bool
	}{
		{
			name:      "bare",
			tool:      "exec_command",
			arguments: `{"cmd":"backscroll search --text orchard"}`,
			want:      true,
		},
		{
			name:      "no_args",
			tool:      "exec_command",
			arguments: `{"cmd":"backscroll search"}`,
			want:      true,
		},
		{
			name:      "different_command",
			tool:      "exec_command",
			arguments: `{"cmd":"rg orchard"}`,
			want:      false,
		},
		{
			name:      "malformed_arguments",
			tool:      "exec_command",
			arguments: `not json`,
			want:      false,
		},
		{
			name:      "missing_cmd_key",
			tool:      "exec_command",
			arguments: `{}`,
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCodexDirectSearchCall(tt.tool, tt.arguments); got != tt.want {
				t.Errorf("IsCodexDirectSearchCall(%q, %q) = %v, want %v", tt.tool, tt.arguments, got, tt.want)
			}
		})
	}
}

func TestIsCodexDirectSearchCall_UnknownTool(t *testing.T) {
	if got := IsCodexDirectSearchCall("unknown", `{"command":["sh","-c","backscroll search"]}`); got {
		t.Errorf("unknown tool should not match, got true")
	}
	if got := IsCodexDirectSearchCall("", `{"command":["sh","-c","backscroll search"]}`); got {
		t.Errorf("empty tool should not match, got true")
	}
}
