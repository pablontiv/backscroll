package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/recovery"
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
			markerCalls := 0
			events := []string{}
			policy := func(context.Context, io.Writer) startupResult {
				calls++
				events = append(events, "startup")
				return startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}}
			}
			root := buildRootCmdWithStartup(io.Discard, io.Discard, policy)
			replaceRootCommandRunE(t, root, argv[0], func(cmd *cobra.Command, args []string) error {
				markerCalls++
				events = append(events, "handler")
				return nil
			})
			root.SetArgs(argv)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute %v: %v", argv, err)
			}
			if calls != 1 {
				t.Fatalf("startup calls=%d, want 1", calls)
			}
			if markerCalls != 1 {
				t.Fatalf("handler calls=%d, want 1", markerCalls)
			}
			if want := []string{"startup", "handler"}; !reflect.DeepEqual(events, want) {
				t.Fatalf("events=%v, want %v", events, want)
			}
		})
	}
}

func TestFailedStartupAllowsOnlyRecoverWithInjectedPolicy(t *testing.T) {
	testCases := []struct {
		name   string
		result startupResult
	}{
		{
			name: "error_only",
			result: startupResult{
				Config: &config.Config{DatabasePath: "/tmp/error-only.db"},
				Err:    errors.New("synthetic startup error"),
			},
		},
		{
			name: "diagnostic_and_error",
			result: startupResult{
				Config: &config.Config{DatabasePath: "/tmp/diagnostic.db"},
				Diagnostic: &compat.Diagnostic{
					Code:         compat.CodeIndexStale,
					Summary:      "synthetic startup diagnostic",
					Continuation: []string{"recover", "--from", "/tmp/diagnostic.db", "--dry-run"},
				},
				Err: errors.New("synthetic startup diagnostic error"),
			},
		},
	}

	blockedCommands := []struct {
		name string
		argv []string
	}{
		{name: "search", argv: []string{"search", "needle"}},
		{name: "list", argv: []string{"list"}},
		{name: "config", argv: []string{"config"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			policyCalls := 0
			policy := func(context.Context, io.Writer) startupResult {
				policyCalls++
				return tc.result
			}

			var recoverOut, recoverErr bytes.Buffer
			recoverRoot := buildRootCmdWithStartup(&recoverOut, &recoverErr, policy)
			recoverReached := false
			replaceRootCommandRunE(t, recoverRoot, "recover", func(cmd *cobra.Command, args []string) error {
				recoverReached = true
				assertStartupResultInContext(t, startupResultFrom(cmd), tc.result)
				_, _ = io.WriteString(cmd.OutOrStdout(), "recover-marker\n")
				return nil
			})
			recoverRoot.SetArgs([]string{"recover", "--from", "missing.db", "--dry-run"})
			if err := recoverRoot.Execute(); err != nil {
				t.Fatalf("recover should proceed on startup failure: %v", err)
			}
			if !recoverReached {
				t.Fatal("recover marker was not reached")
			}
			if !strings.Contains(recoverOut.String(), "recover-marker") {
				t.Fatalf("recover marker output missing: stdout=%q stderr=%q", recoverOut.String(), recoverErr.String())
			}
			if policyCalls != 1 {
				t.Fatalf("recover startup calls=%d, want 1", policyCalls)
			}

			for _, blocked := range blockedCommands {
				t.Run("blocks_"+blocked.name, func(t *testing.T) {
					var blockedOut, blockedErr bytes.Buffer
					blockedRoot := buildRootCmdWithStartup(&blockedOut, &blockedErr, policy)
					blockedReached := false
					replaceRootCommandRunE(t, blockedRoot, blocked.name, func(cmd *cobra.Command, args []string) error {
						blockedReached = true
						_, _ = io.WriteString(cmd.OutOrStdout(), "blocked-marker\n")
						return nil
					})
					blockedRoot.SetArgs(blocked.argv)
					err := blockedRoot.Execute()
					if err == nil {
						t.Fatalf("%s unexpectedly succeeded on startup failure; stdout=%q stderr=%q", blocked.name, blockedOut.String(), blockedErr.String())
					}
					if blockedReached {
						t.Fatalf("%s marker should not run; stdout=%q stderr=%q", blocked.name, blockedOut.String(), blockedErr.String())
					}
					combined := blockedOut.String() + blockedErr.String()
					if strings.Contains(combined, "blocked-marker") {
						t.Fatalf("blocked command emitted marker output: stdout=%q stderr=%q", blockedOut.String(), blockedErr.String())
					}
					if tc.result.Diagnostic != nil && !strings.Contains(blockedErr.String(), "diagnostic:") {
						t.Fatalf("expected diagnostic output for blocked command; stdout=%q stderr=%q", blockedOut.String(), blockedErr.String())
					}
				})
			}
			if policyCalls != 1+len(blockedCommands) {
				t.Fatalf("total startup calls=%d, want %d", policyCalls, 1+len(blockedCommands))
			}
		})
	}
}

func TestRecoverAloneContinuesAfterStartupFailure(t *testing.T) {
	startupErr := errors.New("injected startup failure")
	recoveryErr := errors.New("injected recovery failure")
	called := false
	root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}, Err: startupErr}
	})
	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		called = true
		return recovery.Report{}, recoveryErr
	}
	t.Cleanup(func() { recoverExecute = originalExecute })
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err := root.Execute()
	if !called {
		t.Fatal("recover handler did not continue after startup failure")
	}
	if !errors.Is(err, startupErr) {
		t.Fatalf("error=%v does not preserve startup failure", err)
	}
	if !errors.Is(err, recoveryErr) {
		t.Fatalf("error=%v does not preserve recovery failure", err)
	}
}

func TestStartupFailurePreventsHandlerOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	policyErr := errors.New("injected startup failure")
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Err: policyErr}
	})
	root.SetArgs([]string{"config", "--json"})
	err := root.Execute()
	if !errors.Is(err, policyErr) {
		t.Fatalf("error=%v, want injected startup failure", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("handler emitted output after startup failure: %q", stdout.String())
	}
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

func replaceRootCommandRunE(t *testing.T, root *cobra.Command, commandName string, runE func(*cobra.Command, []string) error) {
	t.Helper()
	for _, child := range root.Commands() {
		if child.Name() == commandName {
			child.Run = nil
			child.RunE = runE
			return
		}
	}
	t.Fatalf("root command %q not found", commandName)
}

func assertStartupResultInContext(t *testing.T, got, want startupResult) {
	t.Helper()
	if got.Config != want.Config {
		t.Fatalf("startup config pointer mismatch: got=%p want=%p", got.Config, want.Config)
	}
	if (got.Err == nil) != (want.Err == nil) {
		t.Fatalf("startup err nil mismatch: got=%v want=%v", got.Err, want.Err)
	}
	if got.Err != nil && got.Err.Error() != want.Err.Error() {
		t.Fatalf("startup err=%q, want %q", got.Err.Error(), want.Err.Error())
	}
	if (got.Diagnostic == nil) != (want.Diagnostic == nil) {
		t.Fatalf("startup diagnostic nil mismatch: got=%+v want=%+v", got.Diagnostic, want.Diagnostic)
	}
	if got.Diagnostic != nil {
		if got.Diagnostic.Code != want.Diagnostic.Code || got.Diagnostic.Summary != want.Diagnostic.Summary || !reflect.DeepEqual(got.Diagnostic.Continuation, want.Diagnostic.Continuation) {
			t.Fatalf("startup diagnostic=%+v, want %+v", got.Diagnostic, want.Diagnostic)
		}
	}
}
