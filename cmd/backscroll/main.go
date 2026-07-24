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

// newUpdater is the single wiring point for autoupdate. It is called with no
// envDisable argument, so no environment variable can disable a released binary
// — the only exemption is version=="dev", which picokit applies intrinsically.
// run() and the wiring test both go through here, so the test covers the real
// call site rather than a copy of it.
func newUpdater() *autoupdate.Updater {
	return autoupdate.New("pablontiv/backscroll", "backscroll")
}

func run(stdout, stderr io.Writer, args []string) error {
	u := newUpdater()
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
		newPatternsCmd(stdout, stderr),
		newRebuildCmd(stdout, stderr),
		newPurgeCmd(stdout, stderr),
		newValidateCmd(stdout, stderr),
		newStatusCmd(stdout, stderr),
		newConfigCmd(stdout, stderr),
		newAnnotateCmd(stdout, stderr),
	)

	return root
}
