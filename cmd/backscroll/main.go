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

type processDependencies struct {
	notifySignals func(context.Context) (context.Context, context.CancelFunc)
}

func newProcessSignalNotifier(ready io.Writer) func(context.Context) (context.Context, context.CancelFunc) {
	return func(parent context.Context) (context.Context, context.CancelFunc) {
		ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
		if ready != nil {
			_, _ = ready.Write([]byte{1})
		}
		return ctx, stop
	}
}

// realMain owns the process signal lifecycle. Embedded callers use run or
// runContext directly and therefore never install process-wide handlers.
func realMain(stdout, stderr io.Writer, args []string) int {
	return realMainWithDependencies(stdout, stderr, args, processDependencies{
		notifySignals: newProcessSignalNotifier(nil),
	})
}

func realMainWithDependencies(stdout, stderr io.Writer, args []string, deps processDependencies) int {
	notifySignals := deps.notifySignals
	if notifySignals == nil {
		notifySignals = newProcessSignalNotifier(nil)
	}
	ctx, stop := notifySignals(context.Background())
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

type runDependencies struct {
	applyStaged   func() error
	fetchAndStage func(context.Context, string) error
	root          *cobra.Command
	stagingWait   time.Duration
}

func newRunDependencies(stdout, stderr io.Writer) runDependencies {
	u := newUpdater()
	u.CurrentVersion = version
	return runDependencies{
		applyStaged: u.ApplyStagedIfAvailable,
		// Picokit's fetch API is not context-aware. Its HTTP client bounds the
		// operation, so cancellation detaches this residual worker rather than
		// claiming to stop the underlying network operation immediately.
		fetchAndStage: func(_ context.Context, currentVersion string) error {
			return u.FetchAndStage(currentVersion)
		},
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

	return runContextWithDependencies(ctx, stdout, stderr, args, newRunDependencies(stdout, stderr))
}

func runContextWithDependencies(ctx context.Context, stdout, stderr io.Writer, args []string, deps runDependencies) error {
	root := deps.root
	if root == nil {
		root = buildRootCmd(stdout, stderr)
	}
	stagingWait := deps.stagingWait
	if stagingWait <= 0 {
		stagingWait = autoupdateStagingWait
	}
	if deps.applyStaged != nil {
		_ = deps.applyStaged()
	}

	// A buffered completion notification prevents a detached worker from ever
	// blocking when the caller has already returned. The channel is also closed
	// so completion remains observable after the buffered value is consumed.
	staged := make(chan struct{}, 1)
	go func() {
		defer close(staged)
		if deps.fetchAndStage != nil {
			_ = deps.fetchAndStage(ctx, version)
		}
		staged <- struct{}{}
	}()

	if indexPolicyMachineArgs(args) {
		root.SilenceUsage = true
	}
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		return err
	}

	// Wait for staging to complete so short-lived commands don't kill the
	// download before it finishes. Cancellation returns the caller promptly but
	// the production Picokit fetch can remain as a bounded detached worker.
	timer := time.NewTimer(stagingWait)
	waitErr := waitForStaging(ctx, staged, timer.C)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	return waitErr
}

func waitForStaging(ctx context.Context, staged <-chan struct{}, timeout <-chan time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-staged:
	case <-timeout:
	case <-ctx.Done():
	}
	return ctx.Err()
}
