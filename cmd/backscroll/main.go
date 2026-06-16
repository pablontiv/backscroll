package main

import (
	"fmt"
	"io"
	"os"
	"time"

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

	staged := make(chan struct{})
	go func() {
		defer close(staged)
		_ = u.FetchAndStage(version)
	}()

	rootCmd := buildRootCmd(stdout, stderr)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()

	// Wait for staging to complete so short-lived commands don't kill the
	// download before it finishes. Output is already on screen; process lingers
	// silently for at most 10s on slow connections.
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-staged:
	case <-timer.C:
	}

	return err
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
		newSearchCmd(stdout, stderr),
		newReadCmd(stdout, stderr),
		newListCmd(stdout, stderr),
		newStatsCmd(stdout, stderr),
		newRebuildCmd(stdout, stderr),
		newPurgeCmd(stdout, stderr),
		newValidateCmd(stdout, stderr),
		newStatusCmd(stdout, stderr),
		newConfigCmd(stdout, stderr),
	)

	return root
}
