package readers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func TestCodexNativeToolOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, tool, kind, args, input string
		output                        any
		native                        *bool
		wantError                     *bool
		wantCode                      *int
		wantHead                      string
	}{
		{name: "shell success", tool: "exec_command", args: `{"cmd":"go test ./..."}`, output: "Chunk ID: abc\nWall time: 0.1 seconds\nProcess exited with code 0\nOutput:\nok", wantHead: "go", wantError: new(false), wantCode: new(0)},
		{name: "shell failure", tool: "shell", args: `{"command":["/bin/sh","-lc","MODE=test go test"]}`, output: "Process exited with code 7\nOutput:\nerror: failed", wantHead: "go", wantError: new(true), wantCode: new(7)},
		{name: "argv", tool: "shell", args: `{"command":["git","status"]}`, output: "unknown", wantHead: "git"},
		{name: "raw long metadata", tool: "exec_command", args: `{"cmd":"make test"}`, output: `{"output":"` + strings.Repeat("x", MaxToolTextLen+100) + `","metadata":{"exit_code":9}}`, wantHead: "make", wantError: new(true), wantCode: new(9)},
		{name: "raw object metadata", tool: "exec_command", args: `{"cmd":"echo ok"}`, output: map[string]any{"metadata": map[string]any{"exit_code": 0}}, wantHead: "echo", wantError: new(false), wantCode: new(0)},
		{name: "explicit false", tool: "apply_patch", kind: "custom_tool_call", input: "patch", output: "error text is not metadata", native: new(false), wantError: new(false)},
		{name: "explicit true empty output", tool: "apply_patch", kind: "custom_tool_call", input: "patch", output: "", native: new(true), wantError: new(true)},
		{name: "missing signal", tool: "apply_patch", kind: "custom_tool_call", input: "patch", output: "Done"},
		{name: "missing result", tool: "exec_command", args: `{"cmd":"false"}`, wantHead: "false"},
		{name: "wrapper success describes wrapper only", tool: "exec", kind: "custom_tool_call", input: `await tool.bash({command:"false"})`, output: "Script completed\n{\"isError\":true,\"exit_code\":7}", wantError: new(false)},
		{name: "wrapper failure", tool: "exec", kind: "custom_tool_call", input: `throw new Error("no")`, output: "Script failed\nerror", wantError: new(true)},
		{name: "wrapped unknown", tool: "exec", args: `{"command":"const result = await tool.bash({command: 'false'})"}`, output: `{"metadata":{"exit_code":0},"result":{"is_error":false}}`},
		{name: "unsupported response", tool: "unknown_tool", args: `{"cmd":"false","command":"false"}`, output: "Process exited with code 0\nOutput:\nnot a shell"},
		{name: "quoted exit", tool: "exec_command", args: `{"cmd":"cat log"}`, output: "Process exited with code 0\nOutput:\nProcess exited with code 42", wantHead: "cat", wantError: new(false), wantCode: new(0)},
		{name: "arbitrary output", tool: "exec_command", args: `{"cmd":"cat log"}`, output: "some text\nProcess exited with code 42\nOutput:\nquoted", wantHead: "cat"},
		{name: "bare exit prose", tool: "exec_command", args: `{"cmd":"cat log"}`, output: "Process exited with code 42", wantHead: "cat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind := tc.kind
			if kind == "" {
				kind = "function_call"
			}
			call := map[string]any{"type": kind, "name": tc.tool, "call_id": "native-call", "arguments": tc.args, "input": tc.input}
			outputKind := "function_call_output"
			if kind == "custom_tool_call" {
				outputKind = "custom_tool_call_output"
			}
			result := map[string]any{"type": outputKind, "call_id": "native-call", "output": tc.output}
			if tc.native != nil {
				result["is_error"] = *tc.native
			}
			var lines strings.Builder
			// Result first proves linkage does not depend on adjacency or order.
			if tc.output != nil {
				codexWriteTestRecord(t, &lines, result)
			}
			codexWriteTestRecord(t, &lines, map[string]any{"type": "function_call", "name": "unrelated", "call_id": "other", "arguments": "{}"})
			codexWriteTestRecord(t, &lines, call)
			parsed, err := (&CodexReader{}).Parse(context.Background(), codexTestFile(t, lines.String()), input_config.InputDefinition{})
			if err != nil {
				t.Fatal(err)
			}
			var events int
			for _, msg := range parsed.Records {
				if msg.ToolName == "" {
					if msg.Role != "tool" {
						t.Fatalf("unexpected non-call: %+v", msg)
					}
					continue
				}
				events++
				if msg.ToolUseID == "other" {
					if msg.IsError != nil {
						t.Fatal("interleaved result contaminated other call")
					}
					continue
				}
				if msg.ToolName != tc.tool || msg.ToolUseID != "native-call" || msg.UUID != "" || msg.CommandHead != tc.wantHead {
					t.Fatalf("identity/head: %+v", msg)
				}
				if !codexEqualPtr(msg.IsError, tc.wantError) || !codexEqualPtr(msg.ExitCode, tc.wantCode) {
					t.Fatalf("outcome: %+v want error=%v code=%v", msg, tc.wantError, tc.wantCode)
				}
			}
			if events != 2 {
				t.Fatalf("events=%d, want exactly two calls", events)
			}
		})
	}
}

func codexWriteTestRecord(t *testing.T, out *strings.Builder, payload map[string]any) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"type": "response_item", "timestamp": "2026-09-01T12:00:00Z", "payload": payload})
	if err != nil {
		t.Fatal(err)
	}
	out.Write(data)
	out.WriteByte('\n')
}

func codexEqualPtr[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
