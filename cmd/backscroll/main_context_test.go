package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func testRunDependencies(root *cobra.Command, fetch func(context.Context, string) error) runDependencies {
	return runDependencies{root: root, fetchAndStage: fetch, stagingWait: autoupdateStagingWait}
}

func runWithTestDependencies(ctx context.Context, args []string, deps runDependencies) error {
	return runContextWithDependencies(ctx, new(bytes.Buffer), new(bytes.Buffer), args, deps)
}

func TestRunBackgroundWrapperCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "help", args: []string{"--help"}},
		{name: "version", args: []string{"--version"}},
		{name: "skill", args: []string{"--skill"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var runOut, runErrOut bytes.Buffer
			runErr := run(&runOut, &runErrOut, tc.args)

			var contextOut, contextErrOut bytes.Buffer
			contextErr := runContext(context.Background(), &contextOut, &contextErrOut, tc.args)

			if !bytes.Equal(runOut.Bytes(), contextOut.Bytes()) {
				t.Fatalf("stdout differs\nrun: %q\nrunContext: %q", runOut.String(), contextOut.String())
			}
			if !bytes.Equal(runErrOut.Bytes(), contextErrOut.Bytes()) {
				t.Fatalf("stderr differs\nrun: %q\nrunContext: %q", runErrOut.String(), contextErrOut.String())
			}
			if runErr != contextErr {
				t.Fatalf("errors differ: run=%v runContext=%v", runErr, contextErr)
			}
		})
	}
}

func TestRunContextDependenciesAreNilSafe(t *testing.T) {
	var root *cobra.Command
	var apply func() error
	var fetch func(context.Context, string) error
	var stdout, stderr bytes.Buffer
	err := runContextWithDependencies(context.Background(), &stdout, &stderr, []string{"--help"}, runDependencies{
		root: root, applyStaged: apply, fetchAndStage: fetch,
	})
	if err != nil {
		t.Fatalf("nil dependencies: %v", err)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("backscroll")) {
		t.Fatalf("default root help missing from stdout: %q", stdout.String())
	}
}

func TestRunContextCancellationReachesStartupAndHandler(t *testing.T) {
	t.Run("startup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		startupContext := make(chan context.Context, 1)
		handlerCalled := false
		root := &cobra.Command{
			Use: "root",
			PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
				startupContext <- cmd.Context()
				return cmd.Context().Err()
			},
			RunE: func(*cobra.Command, []string) error {
				handlerCalled = true
				return nil
			},
		}

		err := runWithTestDependencies(ctx, nil, testRunDependencies(root, nil))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v want context.Canceled", err)
		}
		if got := <-startupContext; !errors.Is(got.Err(), context.Canceled) {
			t.Fatalf("startup context error=%v want context.Canceled", got.Err())
		}
		if handlerCalled {
			t.Fatal("handler ran after canceled startup")
		}
	})

	t.Run("handler", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		handlerStarted := make(chan struct{})
		startupContext := make(chan context.Context, 1)
		handlerContext := make(chan context.Context, 1)
		root := &cobra.Command{
			Use: "root",
			PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
				startupContext <- cmd.Context()
				return nil
			},
			RunE: func(cmd *cobra.Command, _ []string) error {
				close(handlerStarted)
				<-cmd.Context().Done()
				handlerContext <- cmd.Context()
				return cmd.Context().Err()
			},
		}

		result := make(chan error, 1)
		go func() {
			result <- runWithTestDependencies(ctx, nil, testRunDependencies(root, nil))
		}()
		<-handlerStarted
		if got := <-startupContext; got != ctx {
			t.Fatal("startup did not receive ExecuteContext context")
		}
		cancel()

		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("handler did not return after cancellation")
		}
		if got := <-handlerContext; !errors.Is(got.Err(), context.Canceled) {
			t.Fatalf("handler context error=%v want context.Canceled", got.Err())
		}
	})
}

type cancelOnSecondCheckContext struct {
	context.Context
	checks int
}

func (c *cancelOnSecondCheckContext) Done() <-chan struct{} { return nil }

func (c *cancelOnSecondCheckContext) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.Canceled
	}
	return nil
}

func TestWaitForStagingCancellationDominatesReadyBranches(t *testing.T) {
	const iterations = 1000
	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		staged := make(chan struct{}, 1)
		staged <- struct{}{}
		if err := waitForStaging(ctx, staged, make(chan time.Time)); !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-canceled staged-ready iteration %d: error=%v", i, err)
		}

		ctx, cancel = context.WithCancel(context.Background())
		cancel()
		timer := make(chan time.Time, 1)
		timer <- time.Time{}
		if err := waitForStaging(ctx, make(chan struct{}), timer); !errors.Is(err, context.Canceled) {
			t.Fatalf("pre-canceled timer-ready iteration %d: error=%v", i, err)
		}

		postCtx := &cancelOnSecondCheckContext{Context: context.Background()}
		staged = make(chan struct{}, 1)
		staged <- struct{}{}
		if err := waitForStaging(postCtx, staged, make(chan time.Time)); !errors.Is(err, context.Canceled) {
			t.Fatalf("post-canceled staged-selected iteration %d: error=%v", i, err)
		}

		postCtx = &cancelOnSecondCheckContext{Context: context.Background()}
		timer = make(chan time.Time, 1)
		timer <- time.Time{}
		if err := waitForStaging(postCtx, make(chan struct{}), timer); !errors.Is(err, context.Canceled) {
			t.Fatalf("post-canceled timer-selected iteration %d: error=%v", i, err)
		}
	}
}

func TestRunContextCanceledStagingWorkerExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fetchStarted := make(chan struct{})
	workerExited := make(chan struct{})
	commandDone := make(chan struct{})
	root := &cobra.Command{Use: "root", RunE: func(*cobra.Command, []string) error {
		close(commandDone)
		return nil
	}}
	fetch := func(ctx context.Context, _ string) error {
		close(fetchStarted)
		defer close(workerExited)
		<-ctx.Done()
		return ctx.Err()
	}

	result := make(chan error, 1)
	go func() {
		result <- runWithTestDependencies(ctx, nil, testRunDependencies(root, fetch))
	}()
	<-fetchStarted
	<-commandDone
	select {
	case err := <-result:
		t.Fatalf("blocked staging wait returned before cancellation: %v", err)
	default:
	}

	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked staging wait did not return after cancellation")
	}
	select {
	case <-workerExited:
	case <-time.After(2 * time.Second):
		t.Fatal("context-aware fake staging worker did not exit")
	}
}

func TestRunContextStillWaitsForSuccessfulStaging(t *testing.T) {
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	commandDone := make(chan struct{})
	root := &cobra.Command{Use: "root", RunE: func(*cobra.Command, []string) error {
		close(commandDone)
		return nil
	}}
	fetch := func(context.Context, string) error {
		close(fetchStarted)
		<-releaseFetch
		return nil
	}

	result := make(chan error, 1)
	go func() {
		result <- runWithTestDependencies(context.Background(), nil, testRunDependencies(root, fetch))
	}()
	<-fetchStarted
	<-commandDone
	select {
	case err := <-result:
		close(releaseFetch)
		t.Fatalf("run returned before successful staging completed: %v", err)
	default:
	}

	close(releaseFetch)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("run error after successful staging: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return after successful staging")
	}
}

type countingStartupLease struct {
	releases int
}

func (l *countingStartupLease) Release() error {
	l.releases++
	return nil
}

func TestRunContextCancellationReleasesCommandLease(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lease := &countingStartupLease{}
	handlerStarted := make(chan struct{})
	root := &cobra.Command{
		Use: "root",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SetContext(context.WithValue(cmd.Context(), startupContextKey{}, startupResult{Lease: lease}))
			return nil
		},
	}
	registerStartupCommand(root, startupMutation, &cobra.Command{
		Use: "mutate",
		RunE: func(cmd *cobra.Command, _ []string) error {
			close(handlerStarted)
			<-cmd.Context().Done()
			return cmd.Context().Err()
		},
	})

	result := make(chan error, 1)
	go func() {
		result <- runWithTestDependencies(ctx, []string{"mutate"}, testRunDependencies(root, nil))
	}()
	<-handlerStarted
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mutating command did not return after cancellation")
	}
	if lease.releases != 1 {
		t.Fatalf("lease releases=%d want 1", lease.releases)
	}
}
