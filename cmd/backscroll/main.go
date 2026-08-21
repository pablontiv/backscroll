package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pablontiv/picokit/autoupdate"
)

var version = "dev"

func main() {
	if err := run(os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		if !diagnosticAlreadyRendered(err) {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func diagnosticAlreadyRendered(err error) bool {
	_, ok := err.(indexDiagnosticError)
	return ok
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
