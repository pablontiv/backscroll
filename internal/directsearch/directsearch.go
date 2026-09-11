// Package directsearch holds the shared predicates the Codex ingest path
// and the storage replay path both rely on to decide whether a stored tool
// call is a direct `backscroll search` invocation. Keeping a single source of
// truth here is the only way the SQL "what's pending requeue?" predicate and
// the reader's "did this row get marked?" predicate can stay in lockstep —
// every prior SQL GLOB attempt to enumerate separator byte-sequences missed at
// least one real encoding (Unicode whitespace that strings.Fields accepts but
// JSON escapes in non-uniform ways), so the replay check now decodes the JSON
// argv and re-runs the exact strings.Fields-based acceptance that ingest
// already uses.
package directsearch

import (
	"encoding/json"
	"path"
	"strings"
)

// IsDirectSearchCommand is the one command boundary every reader shares: the
// raw command text must start with the bare `backscroll search` tokens.
// Absolute paths, env/shell wrappers, and other subcommands are not echoes.
// It mirrors strings.Fields splitting, so any unicode.IsSpace separator
// between the two tokens is accepted.
func IsDirectSearchCommand(command string) bool {
	fields := strings.Fields(command)
	return len(fields) >= 2 && fields[0] == "backscroll" && fields[1] == "search"
}

// IsCodexDirectSearchCall recognizes Codex's own direct shell invocations of
// `backscroll search`: an `exec_command` whose raw `cmd` starts with the bare
// tokens, or a `shell` call whose argv is exactly a shell, `-c`/`-lc`, and
// that same command string. `arguments` is the JSON object the codex rollout
// stored (e.g. `{"command":["sh","-c","backscroll search"]}` or
// `{"cmd":"backscroll search"}`).
func IsCodexDirectSearchCall(tool, arguments string) bool {
	switch tool {
	case "exec_command":
		var obj struct {
			Cmd string `json:"cmd"`
		}
		if json.Unmarshal([]byte(arguments), &obj) != nil {
			return false
		}
		return IsDirectSearchCommand(obj.Cmd)
	case "shell":
		var obj struct {
			Command []string `json:"command"`
		}
		if json.Unmarshal([]byte(arguments), &obj) != nil || len(obj.Command) != 3 {
			return false
		}
		if !strings.HasSuffix(path.Base(obj.Command[0]), "sh") {
			return false
		}
		if obj.Command[1] != "-c" && obj.Command[1] != "-lc" {
			return false
		}
		return IsDirectSearchCommand(obj.Command[2])
	}
	return false
}
