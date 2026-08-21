package main

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestEveryOperationalCommandHasApprovedStartupClass(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	want := map[string]startupCommandClass{
		"search":   startupSnapshotRead,
		"list":     startupSnapshotRead,
		"patterns": startupSnapshotRead,
		"status":   startupSnapshotRead,
		"validate": startupSnapshotRead,
		"config":   startupMetadataRead,
		"annotate": startupMutation,
		"purge":    startupMutation,
		"rebuild":  startupMutation,
		"recover":  startupMutation,
	}
	if len(root.Commands()) != len(want) {
		t.Fatalf("operational command count=%d want=%d", len(root.Commands()), len(want))
	}
	for _, command := range root.Commands() {
		got, explicit := startupCommandClassFor(command)
		if !explicit {
			t.Errorf("%s has no explicit startup class", command.Name())
			continue
		}
		if got != want[command.Name()] {
			t.Errorf("%s class=%q want=%q", command.Name(), got, want[command.Name()])
		}
	}
}

func TestMutationRegistrationReleasesStartupLease(t *testing.T) {
	handlerErr := errors.New("handler failed")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "success"},
		{name: "handler error", err: handlerErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lease := &fakeStartupLease{}
			cmd := &cobra.Command{
				Use: "mutate",
				RunE: func(*cobra.Command, []string) error {
					return tc.err
				},
			}
			root := &cobra.Command{Use: "root"}
			registerStartupCommand(root, startupMutation, cmd)
			cmd.SetContext(context.WithValue(context.Background(), startupContextKey{}, startupResult{Lease: lease}))

			err := cmd.RunE(cmd, nil)
			if !errors.Is(err, tc.err) {
				t.Fatalf("RunE error=%v, want %v", err, tc.err)
			}
			if lease.releases != 1 {
				t.Fatalf("lease releases=%d, want 1", lease.releases)
			}
		})
	}
}

func TestMutationRegistrationJoinsLeaseReleaseError(t *testing.T) {
	releaseErr := errors.New("release failed")
	handlerErr := errors.New("handler failed")
	lease := &fakeStartupLease{err: releaseErr}
	cmd := &cobra.Command{
		Use: "mutate",
		RunE: func(*cobra.Command, []string) error {
			return handlerErr
		},
	}
	root := &cobra.Command{Use: "root"}
	registerStartupCommand(root, startupMutation, cmd)
	cmd.SetContext(context.WithValue(context.Background(), startupContextKey{}, startupResult{Lease: lease}))

	err := cmd.RunE(cmd, nil)
	if !errors.Is(err, handlerErr) {
		t.Fatalf("RunE error=%v does not include handler error", err)
	}
	if !errors.Is(err, releaseErr) {
		t.Fatalf("RunE error=%v does not include lease release error", err)
	}
	if lease.releases != 1 {
		t.Fatalf("lease releases=%d, want 1", lease.releases)
	}
}

func TestReadSafeRegistrationDoesNotReleaseStartupLease(t *testing.T) {
	lease := &fakeStartupLease{}
	cmd := &cobra.Command{
		Use: "read",
		RunE: func(*cobra.Command, []string) error {
			return nil
		},
	}
	root := &cobra.Command{Use: "root"}
	registerStartupCommand(root, startupSnapshotRead, cmd)
	cmd.SetContext(context.WithValue(context.Background(), startupContextKey{}, startupResult{Lease: lease}))

	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE returned error: %v", err)
	}
	if lease.releases != 0 {
		t.Fatalf("lease releases=%d, want 0 for read-safe command", lease.releases)
	}
}

func TestUnknownStartupCommandClassDefaultsRuntimeToMutation(t *testing.T) {
	got, explicit := startupCommandClassFor(&cobra.Command{Use: "unknown"})
	if explicit {
		t.Fatal("unknown command reported explicit startup class")
	}
	if got != startupMutation {
		t.Fatalf("unknown command class=%q, want %q", got, startupMutation)
	}
}

type fakeStartupLease struct {
	releases int
	err      error
}

func (l *fakeStartupLease) Release() error {
	l.releases++
	return l.err
}
