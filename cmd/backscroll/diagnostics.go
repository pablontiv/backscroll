package main

import (
	"fmt"
	"io"
	"strings"
)

// writeSearchHints prints actionable suggestions to w after a query returned zero
// rows. It writes only to w (stderr) so STDOUT — including --json — stays a clean,
// parseable empty payload. allProjects suppresses the --all-projects suggestion;
// alreadyToolScoped suppresses the --content-type tool suggestion.
func writeSearchHints(w io.Writer, allProjects, alreadyToolScoped bool) {
	fmt.Fprintln(w, "no results — suggestions:")
	if !allProjects {
		fmt.Fprintln(w, "  • --all-projects: search across every project, not just the current one")
	}
	if !alreadyToolScoped {
		fmt.Fprintln(w, "  • --content-type tool: match commands, file paths, and errors")
	}
	fmt.Fprintln(w, "  • backscroll status: confirm the index is up to date")
}

// warnShortToolQuery warns when a tool-scoped query is too short for the tool_fts
// trigram tokenizer, which needs ≥3 characters and will otherwise match nothing.
// Self-guarding: does nothing unless contentType == "tool" and the trimmed query
// is under 3 runes. The query still runs; this is advisory only.
func warnShortToolQuery(w io.Writer, contentType, query string) {
	if contentType != "tool" {
		return
	}
	if len([]rune(strings.TrimSpace(query))) < 3 {
		fmt.Fprintf(w, "warning: %q is under 3 characters; the tool index (trigram) needs ≥3 and will match nothing\n", strings.TrimSpace(query))
	}
}
