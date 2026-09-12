// Package directsearch holds the shared predicates every path that must
// answer "is this a direct `backscroll search` call" relies on: the readers'
// ingest-time marking, the storage page exclusion, the --relax IDF counting,
// and the requeue detection. The serialized-text boundary has exactly one
// owner — IsSerializedDirectSearchCall — and SQL only ever applies a broad,
// provable-superset prefilter (the stored text of an accepted call always
// contains the literal substring "backscroll"); the strict decision is made
// in Go by this package. Hand-written SQL pattern equivalents were tried and
// abandoned: enumerating the separator byte-sequences strings.Fields accepts
// is unbounded (Unicode whitespace such as NBSP, U+2028/2029, U+3000; JSON
// control escapes in the shell argv), so per-path SQL recognizers always
// diverged from the Go predicate on some encoding.
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

// IsSerializedDirectSearchCall reports whether serialized tool-input text, as
// produced by readers.SerializeToolInput, is a direct `backscroll search`
// call. It owns all three stored shapes, so the page exclusion, the --relax
// IDF counting, and the requeue detection can share one strict predicate
// behind a broad SQL prefilter instead of re-deriving the boundary per path:
//
//   - bash:         "Bash command=backscroll search ..." (tool-name token
//     case-insensitive, key and subcommand exact)
//   - exec_command: "exec_command cmd=backscroll search ..." (all tokens exact)
//   - shell:        "shell ... command=[\"<shell>\",\"-c\"|\"-lc\",\"<cmd>\"] ..."
//     (Codex wrapper; the argv is JSON-decoded and fed to the same
//     IsCodexDirectSearchCall predicate the reader applies at ingest)
//
// Separators are whatever strings.Fields accepts, in every shape. The shell
// decode deliberately has no token-count floor: an argv whose separators are
// all JSON control escapes serializes to just two whitespace-separated tokens.
func IsSerializedDirectSearchCall(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	switch {
	case strings.EqualFold(fields[0], "bash"):
		return len(fields) >= 3 && fields[1] == "command=backscroll" && fields[2] == "search"
	case fields[0] == "exec_command":
		return len(fields) >= 3 && fields[1] == "cmd=backscroll" && fields[2] == "search"
	case strings.EqualFold(fields[0], "shell"):
		return serializedShellMatches(text)
	}
	return false
}

// serializedShellMatches is the serialized-text half of the Codex shell
// wrapper boundary. SerializeToolInput emits the rollout's `arguments` as a
// space-joined `key=value` token list with keys sorted alphabetically. Real
// Codex shell calls carry not just `command` but also `workdir`,
// `timeout_ms`, and potentially `additional_permissions` or `sandbox` — so
// `command=` is not guaranteed to be the first key. We locate the ` command=`
// token boundary anywhere in the text and feed only what follows to a
// json.Decoder, which stops after reading one complete JSON value (the
// array). Whatever (already-serialized, non-JSON) `key=value` text follows
// the array is ignored. The decoded array is then wrapped into the object
// shape IsCodexDirectSearchCall expects and fed to the exact same predicate
// the reader uses at ingest time.
func serializedShellMatches(text string) bool {
	const token = " command="
	idx := strings.Index(text, token)
	if idx < 0 {
		return false
	}
	remainder := text[idx+len(token):]
	var commandArray []string
	if err := json.NewDecoder(strings.NewReader(remainder)).Decode(&commandArray); err != nil {
		return false
	}
	args, err := json.Marshal(map[string][]string{"command": commandArray})
	if err != nil {
		return false
	}
	return IsCodexDirectSearchCall("shell", string(args))
}
