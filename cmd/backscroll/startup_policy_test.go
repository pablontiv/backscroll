package main

import (
	"context"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/spf13/cobra"
)

func TestEveryOperationalCommandRunsStartupBeforeHandler(t *testing.T) {
	commands := [][]string{
		{"search", "needle"}, {"list"}, {"patterns", "--kind", "commands"},
		{"annotate", "--uuid", "u", "--kind", "correction", "--label", "x"},
		{"purge", "--before", "2030-01-01"}, {"rebuild"}, {"status"},
		{"validate"}, {"config"}, {"recover", "--from", "missing.db", "--dry-run"},
	}
	for _, argv := range commands {
		t.Run(strings.Join(argv, "_"), func(t *testing.T) {
			calls := 0
			policy := func(context.Context, io.Writer) startupResult {
				calls++
				return startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}}
			}
			root := buildRootCmdWithStartup(io.Discard, io.Discard, policy)
			root.SetArgs(argv)
			_ = root.Execute()
			if calls != 1 {
				t.Fatalf("startup calls=%d, want 1", calls)
			}
		})
	}

	t.Run("startup_before_handler", func(t *testing.T) {
		var events []string
		root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
			events = append(events, "startup")
			return startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}}
		})
		root.AddCommand(&cobra.Command{
			Use: "synthetic",
			RunE: func(cmd *cobra.Command, args []string) error {
				events = append(events, "handler")
				return nil
			},
		})
		root.SetArgs([]string{"synthetic"})
		if err := root.Execute(); err != nil {
			t.Fatalf("synthetic command failed: %v", err)
		}
		if want := []string{"startup", "handler"}; !reflect.DeepEqual(events, want) {
			t.Fatalf("events=%v, want %v", events, want)
		}
	})
}

func TestDefaultStartupPolicyCallsSyncExactlyOnce(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	setIndexPolicyEnv(t, dbPath, t.TempDir())
	calls := 0
	originalSync := startupSync
	startupSync = func(*config.Config, io.Writer) error {
		calls++
		return nil
	}
	t.Cleanup(func() { startupSync = originalSync })

	result := defaultStartupPolicy(context.Background(), io.Discard)
	if result.Err != nil || result.Diagnostic != nil {
		t.Fatalf("startup result=%+v", result)
	}
	if calls != 1 {
		t.Fatalf("sync calls=%d, want 1", calls)
	}
}

func TestMetadataCommandsSkipStartup(t *testing.T) {
	for _, argv := range [][]string{{"--help"}, {"--version"}, {"search", "--help"}} {
		calls := 0
		root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
			calls++
			return startupResult{}
		})
		root.SetArgs(argv)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", argv, err)
		}
		if calls != 0 {
			t.Fatalf("%v invoked startup %d times", argv, calls)
		}
	}
}

func TestRootExcludesDirectReadCommand(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	for _, cmd := range root.Commands() {
		if cmd.Name() == "read" {
			t.Fatal("public read command is registered")
		}
	}
}

func TestCommandTreeExcludesIndexedOnlyFlag(t *testing.T) {
	root := buildRootCmd(io.Discard, io.Discard)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if flag := cmd.Flags().Lookup("indexed-only"); flag != nil {
			t.Errorf("%s registers forbidden --indexed-only", cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}
