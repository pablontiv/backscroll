package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestUpdaterInvalidInvocationsDoNoWork(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--version"}, {"help"}, {"help", "config"}, {"config", "--help"}, {"--skill"}, {"--skill=bad"}, {"missing"}, {"config", "--missing"}, {"config", "--no-sync"}, {"annotate"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, cleanup := testEnv(t)
			defer cleanup()
			var apply, fetch atomic.Int32
			var out, errOut bytes.Buffer
			_ = runContextWithDependencies(context.Background(), &out, &errOut, args, runDependencies{
				applyStaged:   func() error { apply.Add(1); return nil },
				fetchAndStage: func(context.Context, string) error { fetch.Add(1); return nil },
				stagingTimer: func(time.Duration) (<-chan time.Time, func()) {
					t.Fatal("invalid invocation waited")
					return nil, func() {}
				},
			})
			if apply.Load() != 0 || fetch.Load() != 0 {
				t.Fatalf("apply=%d fetch=%d", apply.Load(), fetch.Load())
			}
		})
	}
}

func TestUpdaterValidationAndWarnings(t *testing.T) {
	t.Parallel()
	for _, failed := range []bool{false, true} {
		t.Run(string(rune('0'+btoi(failed))), func(t *testing.T) {
			var out, errOut bytes.Buffer
			validated := false
			var apply, fetch atomic.Int32
			commandErr := errors.New("command failed")
			root := &cobra.Command{Use: "root", Args: func(*cobra.Command, []string) error { validated = true; return nil }, PersistentPreRunE: func(*cobra.Command, []string) error {
				if apply.Load() != 1 {
					t.Fatal("updater must start before startup")
				}
				return nil
			}, RunE: func(*cobra.Command, []string) error { out.WriteString(`{"ok":true}`); return commandErr }}
			err := runContextWithDependencies(context.Background(), &out, &errOut, nil, runDependencies{root: root,
				applyStaged: func() error {
					if !validated {
						t.Fatal("apply before Args")
					}
					apply.Add(1)
					if failed {
						return errors.New("apply failed")
					}
					return nil
				},
				fetchAndStage: func(context.Context, string) error {
					fetch.Add(1)
					if failed {
						return errors.New("fetch failed")
					}
					return nil
				},
			})
			if !errors.Is(err, commandErr) || apply.Load() != 1 || fetch.Load() != 1 {
				t.Fatalf("err=%v apply=%d fetch=%d", err, apply.Load(), fetch.Load())
			}
			if out.String() != `{"ok":true}` {
				t.Fatalf("stdout=%q", out.String())
			}
			want := 0
			if failed {
				want = 2
			}
			if strings.Count(errOut.String(), "warning: autoupdate:") != want {
				t.Fatalf("stderr=%q", errOut.String())
			}
		})
	}
}
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestUpdaterRequiredAndFlagGroupsBeforeStartup(t *testing.T) {
	t.Parallel()
	for _, group := range []bool{false, true} {
		var apply atomic.Int32
		root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { t.Fatal("handler ran") }, PersistentPreRun: func(*cobra.Command, []string) { t.Fatal("startup ran") }}
		root.Flags().String("a", "", "")
		root.Flags().String("b", "", "")
		if group {
			root.MarkFlagsRequiredTogether("a", "b")
		} else {
			_ = root.MarkFlagRequired("b")
		}
		err := runWithTestDependencies(context.Background(), []string{"--a=x"}, runDependencies{root: root, applyStaged: func() error { apply.Add(1); return nil }})
		if err == nil || apply.Load() != 0 {
			t.Fatalf("err=%v apply=%d", err, apply.Load())
		}
	}
}

func TestUpdaterStagingCap(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	exited := make(chan struct{})
	tick := make(chan time.Time, 1)
	defer func() { close(release); <-exited }()
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { tick <- time.Time{} }}
	err := runContextWithDependencies(context.Background(), &out, &errOut, nil, runDependencies{root: root, fetchAndStage: func(context.Context, string) error { defer close(exited); <-release; return nil }, stagingTimer: func(time.Duration) (<-chan time.Time, func()) { return tick, func() {} }})
	if err != nil || !strings.Contains(errOut.String(), "staging did not complete within 10s") {
		t.Fatalf("err=%v stderr=%q", err, errOut.String())
	}
}

// A completed fetch cancellation is not a network failure, even when completion
// and cancellation are both ready when the command returns.
func TestUpdaterCanceledCompletionIsSilent(t *testing.T) {
	t.Parallel()
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		var out, errOut bytes.Buffer
		root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { <-done; cancel() }}
		err := runContextWithDependencies(ctx, &out, &errOut, nil, runDependencies{root: root, fetchAndStage: func(context.Context, string) error { defer close(done); return context.Canceled }})
		if !errors.Is(err, context.Canceled) || errOut.Len() != 0 {
			t.Fatalf("err=%v stderr=%q", err, errOut.String())
		}
	}
}

func TestUpdaterMachineOutputUnchanged(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"json", "robot"} {
		var baseline bytes.Buffer
		var wantErr bytes.Buffer
		makeRoot := func(out *bytes.Buffer) *cobra.Command {
			root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { out.WriteString("machine-output\n") }}
			root.Flags().Bool(format, false, "")
			return root
		}
		if err := runContextWithDependencies(context.Background(), &baseline, &wantErr, []string{"--" + format}, runDependencies{root: makeRoot(&baseline)}); err != nil {
			t.Fatal(err)
		}
		var got, errOut bytes.Buffer
		err := runContextWithDependencies(context.Background(), &got, &errOut, []string{"--" + format}, runDependencies{root: makeRoot(&got), applyStaged: func() error { return errors.New("apply failed") }, fetchAndStage: func(context.Context, string) error { return errors.New("fetch failed") }})
		if err != nil || !bytes.Equal(baseline.Bytes(), got.Bytes()) || strings.Count(errOut.String(), "warning: autoupdate:") != 2 {
			t.Fatalf("err=%v stdout=%q stderr=%q", err, got.String(), errOut.String())
		}
	}
}

func TestUpdaterRejectedArgsDoNoWork(t *testing.T) {
	t.Parallel()
	var apply, fetch atomic.Int32
	rejected := errors.New("rejected Args")
	root := &cobra.Command{Use: "root", Args: func(*cobra.Command, []string) error { return rejected }, Run: func(*cobra.Command, []string) { t.Fatal("handler ran") }}
	err := runWithTestDependencies(context.Background(), nil, runDependencies{root: root, applyStaged: func() error { apply.Add(1); return nil }, fetchAndStage: func(context.Context, string) error { fetch.Add(1); return nil }})
	if !errors.Is(err, rejected) || apply.Load() != 0 || fetch.Load() != 0 {
		t.Fatalf("err=%v apply=%d fetch=%d", err, apply.Load(), fetch.Load())
	}
}

func TestUpdaterGeneratedCompletionCommand(t *testing.T) {
	t.Parallel()
	var out, errOut bytes.Buffer
	var apply, fetch atomic.Int32
	root := &cobra.Command{Use: "root"}
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.AddCommand(&cobra.Command{Use: "execute", Run: func(*cobra.Command, []string) {}})
	err := runContextWithDependencies(context.Background(), &out, &errOut, []string{"completion", "bash"}, runDependencies{root: root, applyStaged: func() error { apply.Add(1); return nil }, fetchAndStage: func(context.Context, string) error { fetch.Add(1); return nil }})
	if err != nil || apply.Load() != 1 || fetch.Load() != 1 {
		t.Fatalf("err=%v apply=%d fetch=%d", err, apply.Load(), fetch.Load())
	}
}
