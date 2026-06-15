package main

import (
	"fmt"
	"io"
	"os"

	"github.com/pablontiv/picokit/autoupdate"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	if err := run(os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(stdout, stderr io.Writer, args []string) error {
	u := autoupdate.New("pablontiv/backscroll", "backscroll", "BACKSCROLL_AUTOUPDATE_DISABLE")
	u.CurrentVersion = version
	_ = u.ApplyStagedIfAvailable()
	go u.FetchAndStage(version) //nolint:errcheck

	rootCmd := buildRootCmd(stdout, stderr)
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

func buildRootCmd(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:   "backscroll",
		Short: "Index and search Claude Code sessions",
		Long: `Backscroll is a CLI tool that indexes Claude Code sessions into SQLite
for hybrid full-text search (BM25 + vector embeddings) with RRF fusion.`,
		Version: version,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	root.AddCommand(
		newSyncCmd(stdout, stderr),
		newSearchCmd(stdout, stderr),
		newReadCmd(stdout, stderr),
		newResumeCmd(stdout, stderr),
		newListCmd(stdout, stderr),
		newStatsCmd(stdout, stderr),
		newTopicsCmd(stdout, stderr),
		newInsightsCmd(stdout, stderr),
		newExportCmd(stdout, stderr),
		newRebuildCmd(stdout, stderr),
		newReindexCmd(stdout, stderr),
		newPurgeCmd(stdout, stderr),
		newValidateCmd(stdout, stderr),
		newStatusCmd(stdout, stderr),
		newConfigCmd(stdout, stderr),
		newDecisionsCmd(stdout, stderr),
		newProjectsCmd(stdout, stderr),
		newInputsCmd(stdout, stderr),
		newEventsCmd(stdout, stderr),
		newSessionsCmd(stdout, stderr),
	)

	return root
}
