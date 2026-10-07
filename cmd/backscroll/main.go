package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pablontiv/picokit/autoupdate"
	"github.com/spf13/cobra"
)

const autoupdateStagingWait = 10 * time.Second

var version = "dev"

func main() {
	os.Exit(realMain(os.Stdout, os.Stderr, os.Args[1:]))
}

// realMain owns the process signal lifecycle. Embedded callers use run or
// runContext directly and therefore never install process-wide handlers.
func realMain(stdout, stderr io.Writer, args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runContext(ctx, stdout, stderr, args); err != nil {
		if !diagnosticAlreadyRendered(err) {
			_, _ = fmt.Fprintln(stderr, err)
		}
		return 1
	}
	return 0
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

// stagedUpdater is the minimal autoupdate capability needed by one invocation.
// Production still uses picokit's concrete updater; tests can provide a
// per-call implementation without mutable package-level wiring.
type stagedUpdater interface {
	ApplyStagedIfAvailable() error
	FetchAndStage(string) error
}

type runDependencies struct {
	updater     stagedUpdater
	root        *cobra.Command
	stagingWait time.Duration
}

func newRunDependencies(stdout, stderr io.Writer) runDependencies {
	u := newUpdater()
	u.CurrentVersion = version
	return runDependencies{
		updater:     u,
		root:        buildRootCmd(stdout, stderr),
		stagingWait: autoupdateStagingWait,
	}
}

// run preserves the original embedded API and behavior while the process entry
// point uses runContext to propagate signal cancellation.
func run(stdout, stderr io.Writer, args []string) error {
	return runContext(context.Background(), stdout, stderr, args)
}

func runContext(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	// The skill is informational, self-contained output. Validate and handle its
	// global option before constructing the updater or command tree, while
	// preserving arguments after the POSIX -- separator as command literals.
	printSkill, err := validateSkillInvocation(args)
	if err != nil {
		return err
	}
	if printSkill {
		_, err := io.WriteString(stdout, embeddedBackscrollSkill)
		return err
	}

	return runContextWithDependencies(ctx, args, newRunDependencies(stdout, stderr))
}

func runContextWithDependencies(ctx context.Context, args []string, deps runDependencies) error {
	_ = deps.updater.ApplyStagedIfAvailable()

	staged := make(chan struct{})
	go func() {
		defer close(staged)
		_ = deps.updater.FetchAndStage(version)
	}()

	if indexPolicyMachineArgs(args) {
		deps.root.SilenceUsage = true
	}
	deps.root.SetArgs(args)
	err := deps.root.ExecuteContext(ctx)

	// Wait for staging to complete so short-lived commands don't kill the
	// download before it finishes. Output is already on screen; the process
	// lingers silently for at most 10s on slow connections. Cancellation only
	// stops this wait; picokit's non-context-aware fetch may continue until the
	// process exits.
	timer := time.NewTimer(deps.stagingWait)
	defer timer.Stop()
	select {
	case <-staged:
	case <-timer.C:
	case <-ctx.Done():
		if err == nil {
			return ctx.Err()
		}
	}

	return err
}
