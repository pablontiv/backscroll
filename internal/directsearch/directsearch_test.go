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

func TestIsSerializedDirectSearchCall(t *testing.T) {
	tests := []struct {
		name, text string
		want       bool
	}{
		// bash shape
		{"bash_canonical", "Bash command=backscroll search --text orchard", true},
		{"bash_lowercase", "bash command=backscroll search", true},
		{"bash_uppercase", "BASH command=backscroll search", true},
		{"bash_tab_separator", "Bash command=backscroll\tsearch", true},
		{"bash_nbsp_separator", "Bash command=backscroll search", true},
		{"bash_ideographic_space", "Bash command=backscroll　search", true},
		{"bash_leading_run", "  Bash command=backscroll search", true},
		{"bash_trailing_run", "Bash command=backscroll search  ", true},
		{"bash_status_subcommand", "Bash command=backscroll status", false},
		{"bash_searcher", "Bash command=backscroll searcher", false},
		{"bash_absolute_path", "Bash command=/usr/local/bin/backscroll search", false},
		{"bash_env_wrapper", "Bash command=env backscroll search", false},
		{"bash_nested_lc", `Bash command=bash -lc "backscroll search"`, false},
		{"bash_folded_key", "Bash Command=backscroll search", false},
		{"bash_folded_subcommand", "Bash command=backscroll Search", false},
		{"bash_tool_name_only", "bash", false},
		// exec_command shape
		{"exec_canonical", "exec_command cmd=backscroll search --text orchard", true},
		{"exec_nbsp_separator", "exec_command cmd=backscroll search", true},
		{"exec_status_subcommand", "exec_command cmd=backscroll status", false},
		{"exec_folded_tool_name", "Exec_Command cmd=backscroll search", false},
		// shell shape (Codex wrapper, argv JSON-encoded in the text)
		{"shell_canonical", `shell command=["sh","-c","backscroll search"]`, true},
		{"shell_lc_flag", `shell command=["bash","-lc","backscroll search --text orchard"]`, true},
		{"shell_extra_sorted_keys", `shell additional_permissions=read command=["sh","-c","backscroll search"] timeout_ms=1000`, true},
		{"shell_json_escaped_tab", `shell command=["sh","-c","backscroll\tsearch\t--text\tquery"]`, true},
		{"shell_raw_nbsp_in_argv", "shell command=[\"sh\",\"-c\",\"backscroll search\"]", true},
		{"shell_leading_run", `  shell command=["sh","-c","backscroll search"]`, true},
		{"shell_wrong_flag", `shell command=["sh","-x","backscroll search"]`, false},
		{"shell_extra_argv_element", `shell command=["sh","-c","backscroll search","bar"]`, false},
		{"shell_non_search_call", `shell command=["sh","-c","backscroll status"]`, false},
		{"shell_env_wrapper_in_argv", `shell command=["sh","-c","env backscroll search"]`, false},
		{"shell_no_command_key", "shell workdir=/tmp", false},
		{"shell_malformed_json", "shell command=notjson", false},
		// The argv is stored as raw, un-decoded JSON bytes: a JSON \u-escaped
		// letter of "backscroll" decodes to an accepted command but the stored
		// text lacks the literal substring, so the broad SQL prefilter would
		// miss what the Go predicate accepted. The literal-substring guard
		// keeps the prefilter a provable superset of this predicate.
		{"shell_json_escaped_letter", `shell command=["sh","-c","\u0062ackscroll search --text orchard"]`, false},
		// non-shapes
		{"prose_mention", "run backscroll search from your shell", false},
		{"other_tool", "Bash command=rg backscroll .", false},
		{"empty", "", false},
		{"whitespace_only", " \t ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSerializedDirectSearchCall(tt.text); got != tt.want {
				t.Errorf("IsSerializedDirectSearchCall(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
