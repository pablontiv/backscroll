package main

import (
	"fmt"
	"io"
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
