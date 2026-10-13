package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pablontiv/backscroll/internal/autoupdate"
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
// runtime opt-out argument, so no environment variable can disable a released binary
// — the only exemption is version=="dev", applied by the private backend.
// run() and the wiring test both go through here, so the test covers the real
// call site rather than a copy of it.
func newUpdater() *autoupdate.Updater {
	return autoupdate.New("pablontiv/backscroll", "backscroll")
}

type runDependencies struct {
	applyStaged        func() error
	applyStagedContext func(context.Context) error
	joinFetch          bool
	fetchAndStage      func(context.Context, string) error
	root               *cobra.Command
	stagingWait        time.Duration
	stagingTimer       func(time.Duration) (<-chan time.Time, func())
}

func newRunDependencies(stdout, stderr io.Writer) runDependencies {
	u := newUpdater()
	u.CurrentVersion = version
	return updaterRunDependencies(u, buildRootCmd(stdout, stderr))
}

// updaterRunDependencies is the production adapter, also exercised with safe
// HTTP/filesystem/reexec boundaries in tests.
func updaterRunDependencies(u *autoupdate.Updater, root *cobra.Command) runDependencies {
	return runDependencies{
		applyStagedContext: u.ApplyStagedContext,
		joinFetch:          true,
		fetchAndStage:      u.FetchAndStage,
		root:               root,
		stagingWait:        autoupdateStagingWait,
	}
}

// run preserves the original embedded API and behavior while the process entry
// point uses runContext to propagate signal cancellation.
func run(stdout, stderr io.Writer, args []string) error {
	return runContext(context.Background(), stdout, stderr, args)
}

func runContext(ctx context.Context, stdout, stderr io.Writer, args []string) (retErr error) {
	// Cobra's help callback cannot return an error. Preserve failures at the
	// public run boundary even when a framework output branch discards them.
	output := &requiredOutputWriter{Writer: stdout}
	stdout = output
	defer func() {
		if retErr == nil && output.err != nil {
			retErr = fmt.Errorf("write output: %w", output.err)
		}
	}()

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
	// Keep the informational fast path identical for production and injected runs.
	printSkill, err := validateSkillInvocation(args)
	if err != nil {
		return err
	}
	if printSkill {
		_, err := io.WriteString(stdout, embeddedBackscrollSkill)
		return err
	}
	root := deps.root
	if root == nil {
		root = buildRootCmd(stdout, stderr)
	}
	stagingWait := deps.stagingWait
	if stagingWait <= 0 {
		stagingWait = autoupdateStagingWait
	}
	newTimer := deps.stagingTimer
	if newTimer == nil {
		newTimer = func(d time.Duration) (<-chan time.Time, func()) {
			timer := time.NewTimer(d)
			return timer.C, func() { timer.Stop() }
		}
	}
	warn := func(err error) {
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "warning: autoupdate: %s\n", err)
		}
	}
	fetchContext, cancelFetch := context.WithCancel(ctx)
	defer cancelFetch()
	var staged chan error
	// Cobra calls Args after parsing/command selection and before any pre-run.
	// Required flags and groups normally run AFTER pre-run; move their first
	// check here so neither updating nor ordinary startup can precede validation.
	// Wrap each executable command, not just the root's persistent hook: child
	// hooks may override that hook. Help is informational even though Runnable.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd(args...)
	var install func(*cobra.Command)
	install = func(cmd *cobra.Command) {
		if cmd.Runnable() && cmd.Name() != "help" {
			original := cmd.Args
			cmd.Args = func(cmd *cobra.Command, args []string) error {
				if original != nil {
					if err := original(cmd, args); err != nil {
						return err
					}
				} else if err := cobra.ArbitraryArgs(cmd, args); err != nil {
					return err
				}
				if err := validateRequiredFlagsAndGroups(cmd); err != nil {
					return err
				}
				if ctx.Err() != nil {
					return nil // startup/handler retains cancellation ownership
				}
				if staged == nil {
					if deps.applyStagedContext != nil {
						applyErr := deps.applyStagedContext(ctx)
						if ctx.Err() == nil {
							warn(applyErr)
						}
					} else if deps.applyStaged != nil {
						warn(deps.applyStaged())
					}
					staged = make(chan error, 1)
					go func(done chan<- error) {
						var err error
						if deps.fetchAndStage != nil {
							err = deps.fetchAndStage(fetchContext, version)
						}
						// Only this caller writes warnings; detached completion
						// never touches writers and cannot block on delivery.
						done <- err
					}(staged)
				}
				return nil
			}
		}
		for _, child := range cmd.Commands() {
			install(child)
		}
	}
	install(root)
	if indexPolicyMachineArgs(args) {
		root.SilenceUsage = true
	}
	root.SetArgs(args)
	commandErr := root.ExecuteContext(ctx)
	if staged == nil {
		return commandErr
	}
	timeout, stop := newTimer(stagingWait)
	defer stop()
	completed := false
	var waitErr error
	if ctx.Err() != nil {
		waitErr = ctx.Err()
	} else {
		select {
		case fetchErr := <-staged:
			completed = true
			if ctx.Err() == nil {
				warn(fetchErr)
			}
		case <-timeout:
			if ctx.Err() == nil {
				warn(fmt.Errorf("staging did not complete within 10s"))
			}
		case <-ctx.Done():
		}
		waitErr = ctx.Err()
	}
	if deps.joinFetch && !completed {
		cancelFetch()
		<-staged
	}
	if commandErr != nil {
		return commandErr
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
