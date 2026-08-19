package main

import (
	"errors"
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
		var indexErr indexDiagnosticError
		if !errors.As(err, &indexErr) {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
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
	if indexPolicyMachineArgs(args) {
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
	}
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
		Short: "A permanent, searchable record of your coding-agent sessions",
		Long: `Backscroll turns your coding-agent sessions into a permanent, searchable
record of what happened. It indexes Claude Code, Pi and OpenCode sessions into
SQLite and keeps them after the session files expire.

Prose and tool activity are indexed separately — a Porter-stemmed FTS5 index for
conversation, a trigram index for commands, paths and errors — and an unfiltered
query merges both by rank position (RRF).`,
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
		newRecoverCmd(stdout, stderr),
	)

	return root
}
