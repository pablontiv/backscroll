package storage

import (
	"strings"
)

// ParseToolFromSerialized reverses toolfmt serialization to extract tool_name
// and command_head from INPUTS only. This is lossy (first token only; capped text).
// Used for backfilling tool_events rows for expired-file sessions.
// extraction_version=0 must be set by the caller (signals lossy origin).
//
// Input format (tool_use): "<ToolName> <key1>=<val1> <key2>=<val2> ..."
// Examples:
//
//	"Bash command=go test ..." → ("Bash", "go")
//	"Read file_path=/x.go" → ("Read", "")
//	"Edit file_path=/a old_string=b" → ("Edit", "")
//
// Output format (tool_result, free text): typically no "<key>=" structure.
// Heuristic: input has form "<FirstToken> <key>=..." where FirstToken has no '='.
// If text doesn't match that structure, return ("", "") (signals: skip this row).
//
// NOTE: is_error cannot be reliably extracted from outputs alone (requires tool_use_id
// linkage). Outputs are skipped (toolName="").
func ParseToolFromSerialized(text string) (toolName, commandHead string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}

	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", ""
	}

	// First token is the potential tool_name. It must not contain '=' to be valid.
	firstToken := fields[0]
	if strings.Contains(firstToken, "=") {
		// Looks like "key=value ..." (output or garbage), not a tool name → skip
		return "", ""
	}

	// Check if any subsequent token has '=' (indicates key=value pairs, typical of inputs).
	hasKeyValuePair := false
	for i := 1; i < len(fields); i++ {
		if strings.Contains(fields[i], "=") {
			hasKeyValuePair = true
			break
		}
	}
	if !hasKeyValuePair {
		// No key=value pairs found → likely an output, not a tool input → skip
		return "", ""
	}

	// Matched input structure: extract tool_name and command_head
	toolName = firstToken

	// Extract command_head from "command=" key (if present)
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "command=") {
			cmd := strings.TrimPrefix(f, "command=")
			cmdFields := strings.Fields(cmd)
			if len(cmdFields) > 0 {
				commandHead = cmdFields[0]
			}
			break
		}
	}

	return toolName, commandHead
}
