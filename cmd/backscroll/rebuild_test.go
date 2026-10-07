package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/projects"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/spf13/cobra"
)

const (
	rebuildFTSPrefix      = "Re-deriving FTS indexes from database...\n"
	rebuildBackfillPrefix = "Backfilling derived data from stored text...\n"
	rebuildFallbackPrefix = "Re-resolving projects from session paths...\n"
	rebuildRegistryPrefix = "Checking registry for project label corrections...\n"
	rebuildCompleteLine   = "Rebuild complete. No indexed data was deleted (perennity contract).\n"
)

func TestRebuildRunnerCancellationPrefixes(t *testing.T) {
	tests := []struct {
		name       string
		cancelAt   string
		wantOutput string
		wantCalls  []string
	}{
		{name: "FTS", cancelAt: "fts", wantOutput: rebuildFTSPrefix, wantCalls: []string{"fts"}},
		{name: "backfill", cancelAt: "backfill", wantOutput: rebuildFTSPrefix + rebuildBackfillPrefix, wantCalls: []string{"fts", "backfill"}},
		{name: "fallback", cancelAt: "fallback", wantOutput: rebuildFTSPrefix + rebuildBackfillPrefix + rebuildFallbackPrefix, wantCalls: []string{"fts", "backfill", "fallback"}},
		{name: "registry", cancelAt: "registry", wantOutput: rebuildFTSPrefix + rebuildBackfillPrefix + rebuildFallbackPrefix + rebuildRegistryPrefix, wantCalls: []string{"fts", "backfill", "fallback", "registry"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []string
			phase := func(name string) error {
				calls = append(calls, name)
				if name == tc.cancelAt {
					cancel()
					return ctx.Err()
				}
				return nil
			}
			runner := newRebuildRunner(&rebuildDependencies{
				prepareIndex: func(got context.Context, _ *config.Config, _ indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
					if got != ctx {
						t.Fatal("prepare did not receive command context")
					}
					return nil, nil, nil
				},
				closeIndexDB: func(_ *storage.Database, err error) error { return err },
				rebuildFTSContext: func(_ *storage.Database, got context.Context) error {
					if got != ctx {
						t.Fatal("FTS did not receive command context")
					}
					return phase("fts")
				},
				backfillDerivedContext: func(_ *storage.Database, got context.Context, _ storage.BackfillDerivedOpts) error {
					if got != ctx {
						t.Fatal("backfill did not receive command context")
					}
					return phase("backfill")
				},
				reresolveProjectsContext: func(_ *storage.Database, got context.Context, _ func(string) string) (int64, error) {
					if got != ctx {
						t.Fatal("fallback did not receive command context")
					}
					return 0, phase("fallback")
				},
				loadRegistry: func() projects.ProjectRegistry { return projects.ProjectRegistry{} },
				reresolveProjectsWithRegistryContext: func(_ *storage.Database, got context.Context, _ projects.ProjectRegistry) (int64, error) {
					if got != ctx {
						t.Fatal("registry did not receive command context")
					}
					return 0, phase("registry")
				},
			})
			var stdout, stderr bytes.Buffer
			err := runner.runRebuild(ctx, &stdout, &stderr, &config.Config{})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context.Canceled", err)
			}
			if got := stdout.String(); got != tc.wantOutput {
				t.Fatalf("stdout = %q, want exact prefix %q", got, tc.wantOutput)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if strings.Contains(stdout.String(), "Rebuild complete") {
				t.Fatalf("canceled output contains completion: %q", stdout.String())
			}
			if strings.Join(calls, ",") != strings.Join(tc.wantCalls, ",") {
				t.Fatalf("calls = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}

func TestRebuildRunnerSuccessGoldenAndWriterErrors(t *testing.T) {
	const golden = rebuildFTSPrefix + rebuildBackfillPrefix +
		"Backfill complete: 3 files processed, 4 templates, 5 corrections, 6 lossy events.\n" +
		rebuildFallbackPrefix +
		"Re-resolved 7 sessions with derived project identities.\n" +
		rebuildRegistryPrefix +
		"Registry matched and corrected 8 sessions.\n" +
		rebuildCompleteLine

	newRunner := func() *rebuildRunner {
		return newRebuildRunner(&rebuildDependencies{
			prepareIndex: func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				return nil, nil, nil
			},
			closeIndexDB:      func(_ *storage.Database, err error) error { return err },
			rebuildFTSContext: func(*storage.Database, context.Context) error { return nil },
			backfillDerivedContext: func(_ *storage.Database, _ context.Context, opts storage.BackfillDerivedOpts) error {
				opts.OnProgress(3, 4, 5, 6)
				return nil
			},
			reresolveProjectsContext: func(*storage.Database, context.Context, func(string) string) (int64, error) {
				return 7, nil
			},
			loadRegistry: func() projects.ProjectRegistry { return projects.ProjectRegistry{} },
			reresolveProjectsWithRegistryContext: func(*storage.Database, context.Context, projects.ProjectRegistry) (int64, error) {
				return 8, nil
			},
		})
	}

	t.Run("success", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if err := newRunner().runRebuild(context.Background(), &stdout, &stderr, &config.Config{}); err != nil {
			t.Fatalf("runRebuild: %v", err)
		}
		if got := stdout.String(); got != golden {
			t.Fatalf("stdout mismatch\n got: %q\nwant: %q", got, golden)
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want empty", stderr.String())
		}
	})

	for n := 0; n < len(golden); n++ {
		t.Run("writer_after_"+testInt(n), func(t *testing.T) {
			writeErr := errors.New("injected rebuild writer failure")
			writer := &failAfterWriter{remaining: n, err: writeErr}
			var stderr bytes.Buffer
			err := newRunner().runRebuild(context.Background(), writer, &stderr, &config.Config{})
			if !errors.Is(err, writeErr) {
				t.Fatalf("error = %v, want writer error", err)
			}
			if got, want := writer.String(), golden[:n]; got != want {
				t.Fatalf("output = %q, want golden prefix %q", got, want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRebuildCommandCancellationReleasesMutationLeaseExactlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	lease := &fakeStartupLease{}
	runner := newRebuildRunner(&rebuildDependencies{
		prepareIndex: func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
			return nil, nil, nil
		},
		closeIndexDB: func(_ *storage.Database, err error) error { return err },
		rebuildFTSContext: func(_ *storage.Database, got context.Context) error {
			if got.Done() != ctx.Done() {
				t.Fatal("rebuild did not receive Cobra command cancellation")
			}
			cancel()
			return got.Err()
		},
	})
	var stdout, stderr bytes.Buffer
	root := &cobra.Command{Use: "root"}
	cmd := newRebuildCmd(&stdout, &stderr, runner)
	registerStartupCommand(root, startupMutation, cmd)
	cmd.SetContext(context.WithValue(ctx, startupContextKey{}, startupResult{Config: &config.Config{}, Lease: lease}))

	err := cmd.RunE(cmd, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if lease.releases != 1 {
		t.Fatalf("lease releases = %d, want exactly 1", lease.releases)
	}
	if got := stdout.String(); got != rebuildFTSPrefix {
		t.Fatalf("stdout = %q, want %q", got, rebuildFTSPrefix)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRebuildConstructorsAreNilSafe(t *testing.T) {
	runner := newRebuildRunner(nil)
	if runner == nil || runner.deps.prepareIndex == nil || runner.deps.closeIndexDB == nil ||
		runner.deps.rebuildFTSContext == nil || runner.deps.backfillDerivedContext == nil ||
		runner.deps.reresolveProjectsContext == nil || runner.deps.loadRegistry == nil ||
		runner.deps.reresolveProjectsWithRegistryContext == nil {
		t.Fatalf("newRebuildRunner(nil) left a nil dependency: %+v", runner)
	}
	if cmd := newRebuildCmd(nil, nil, nil); cmd == nil || cmd.RunE == nil {
		t.Fatal("newRebuildCmd with nil arguments did not return a runnable command")
	}
}

func TestRebuildRunnersParallelIsolation(t *testing.T) {
	type result struct {
		name   string
		stdout string
		err    error
	}
	started := make(chan string, 2)
	release := make(chan struct{})
	results := make(chan result, 2)

	start := func(name string, fallback, registry int64) {
		runner := newRebuildRunner(&rebuildDependencies{
			prepareIndex: func(context.Context, *config.Config, indexCommandClass) (*storage.Database, *compat.Diagnostic, error) {
				return nil, nil, nil
			},
			closeIndexDB: func(_ *storage.Database, err error) error { return err },
			rebuildFTSContext: func(*storage.Database, context.Context) error {
				started <- name
				<-release
				return nil
			},
			backfillDerivedContext: func(*storage.Database, context.Context, storage.BackfillDerivedOpts) error { return nil },
			reresolveProjectsContext: func(*storage.Database, context.Context, func(string) string) (int64, error) {
				return fallback, nil
			},
			loadRegistry: func() projects.ProjectRegistry {
				return projects.ProjectRegistry{Projects: []projects.ProjectConfig{{ID: name}}}
			},
			reresolveProjectsWithRegistryContext: func(_ *storage.Database, _ context.Context, got projects.ProjectRegistry) (int64, error) {
				if len(got.Projects) != 1 || got.Projects[0].ID != name {
					return 0, errors.New("runner received another runner's registry")
				}
				return registry, nil
			},
		})
		go func() {
			var stdout bytes.Buffer
			err := runner.runRebuild(context.Background(), &stdout, io.Discard, &config.Config{})
			results <- result{name: name, stdout: stdout.String(), err: err}
		}()
	}

	start("alpha", 11, 12)
	start("beta", 21, 22)
	seen := map[string]bool{<-started: true, <-started: true}
	if !seen["alpha"] || !seen["beta"] {
		t.Fatalf("started runners = %v, want alpha and beta", seen)
	}
	close(release)

	wants := map[string]string{
		"alpha": rebuildFTSPrefix + rebuildBackfillPrefix + rebuildFallbackPrefix +
			"Re-resolved 11 sessions with derived project identities.\n" + rebuildRegistryPrefix +
			"Registry matched and corrected 12 sessions.\n" + rebuildCompleteLine,
		"beta": rebuildFTSPrefix + rebuildBackfillPrefix + rebuildFallbackPrefix +
			"Re-resolved 21 sessions with derived project identities.\n" + rebuildRegistryPrefix +
			"Registry matched and corrected 22 sessions.\n" + rebuildCompleteLine,
	}
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatalf("%s runner: %v", got.name, got.err)
		}
		if got.stdout != wants[got.name] {
			t.Fatalf("%s output = %q, want %q", got.name, got.stdout, wants[got.name])
		}
	}
}

type failAfterWriter struct {
	bytes.Buffer
	remaining int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		return w.Buffer.Write(p)
	}
	n := w.remaining
	w.remaining = 0
	if n > 0 {
		_, _ = w.Buffer.Write(p[:n])
	}
	return n, w.err
}

func testInt(n int) string {
	if n == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
