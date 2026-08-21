package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/recovery"
	"github.com/spf13/cobra"
)

func TestInvalidOperationalCommandsSkipStartup(t *testing.T) {
	testCases := []struct {
		name string
		argv []string
	}{
		{name: "search missing query", argv: []string{"search"}},
		{name: "search invalid fields", argv: []string{"search", "needle", "--fields", "invalid"}},
		{name: "search invalid content type", argv: []string{"search", "needle", "--content-type", "invalid"}},
		{name: "search invalid after", argv: []string{"search", "needle", "--after", "not-a-date"}},
		{name: "search invalid before", argv: []string{"search", "needle", "--before", "not-a-date"}},
		{name: "list positional argument", argv: []string{"list", "unexpected"}},
		{name: "patterns positional argument", argv: []string{"patterns", "unexpected", "--kind", "commands"}},
		{name: "patterns invalid kind", argv: []string{"patterns", "--kind", "invalid"}},
		{name: "patterns invalid trend", argv: []string{"patterns", "--kind", "templates", "--trend"}},
		{name: "patterns conflicting project scope", argv: []string{"patterns", "--kind", "commands", "--project", "p", "--all-projects"}},
		{name: "patterns negative limit", argv: []string{"patterns", "--kind", "commands", "--limit", "-1"}},
		{name: "annotate positional argument", argv: []string{"annotate", "unexpected", "--uuid", "u", "--kind", "correction", "--label", "x"}},
		{name: "annotate missing label", argv: []string{"annotate", "--uuid", "u", "--kind", "correction"}},
		{name: "annotate missing identity", argv: []string{"annotate", "--kind", "correction", "--label", "x"}},
		{name: "purge positional argument", argv: []string{"purge", "unexpected", "--before", "2030-01-01"}},
		{name: "purge missing before", argv: []string{"purge"}},
		{name: "purge invalid before", argv: []string{"purge", "--before", "not-a-date"}},
		{name: "recover missing from", argv: []string{"recover"}},
		{name: "recover empty from", argv: []string{"recover", "--from", ""}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			startupCalls := 0
			root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
				startupCalls++
				return startupResult{Config: &config.Config{DatabasePath: filepath.Join(t.TempDir(), "index.db")}}
			})
			root.SetArgs(tc.argv)

			if err := root.Execute(); err == nil {
				t.Fatalf("execute %v succeeded, want validation error", tc.argv)
			}
			if startupCalls != 0 {
				t.Fatalf("startup calls=%d, want 0 for invalid invocation %v", startupCalls, tc.argv)
			}
		})
	}
}

func TestInvalidSearchDoesNotCreateDatabase(t *testing.T) {
	dir := t.TempDir()
	homeDir := filepath.Join(dir, "home")
	configDir := filepath.Join(dir, "config")
	sessionDir := filepath.Join(dir, "sessions")
	for _, path := range []string{homeDir, configDir, sessionDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
	dbPath := filepath.Join(dir, "index.db")
	t.Setenv("HOME", homeDir)
	t.Setenv("BACKSCROLL_CONFIG_DIR", configDir)
	t.Setenv("BACKSCROLL_DATABASE_PATH", dbPath)
	t.Setenv("BACKSCROLL_SESSION_DIRS", sessionDir)

	root := buildRootCmd(io.Discard, io.Discard)
	root.SetArgs([]string{"search"})
	if err := root.Execute(); err == nil {
		t.Fatal("search without a query succeeded, want validation error")
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("database stat error=%v, want not-exist after invalid invocation", err)
	}
}

func TestEveryOperationalCommandRunsStartupBeforeHandler(t *testing.T) {
	commands := [][]string{
		{"search", "needle"}, {"list"}, {"patterns", "--kind", "commands"},
		{"annotate", "--uuid", "u", "--kind", "correction", "--label", "x"},
		{"purge", "--before", "2030-01-01"},
		{"purge", "--before", "2030-01-01T12:30:00Z"},
		{"rebuild"}, {"status"},
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
			name: "recoverable_sync_error",
			result: startupResult{
				Config: &config.Config{DatabasePath: "/tmp/error-only.db"},
				Failure: &startupFailure{
					Stage:       startupStageStartupSync,
					Cause:       errors.New("synthetic startup error"),
					Diagnostic:  compat.Diagnostic{Code: compat.CodeIndexStale, Summary: "synthetic startup error", Continuation: []string{"recover", "--from", "/tmp/error-only.db", "--dry-run"}},
					Recoverable: true,
				},
			},
		},
		{
			name: "recoverable_diagnostic_and_error",
			result: startupResult{
				Config: &config.Config{DatabasePath: "/tmp/diagnostic.db"},
				Failure: &startupFailure{
					Stage: startupStageIndexPrepare,
					Diagnostic: compat.Diagnostic{
						Code:         compat.CodeIndexStale,
						Summary:      "synthetic startup diagnostic",
						Continuation: []string{"recover", "--from", "/tmp/diagnostic.db", "--dry-run"},
					},
					Cause:       errors.New("synthetic startup diagnostic error"),
					Recoverable: true,
				},
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
					if failure := tc.result.startupFailure(); failure != nil && failure.Diagnostic.Code != "" && !strings.Contains(blockedErr.String(), "diagnostic:") {
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
		dbPath := filepath.Join(t.TempDir(), "active.db")
		return startupResult{Config: &config.Config{DatabasePath: dbPath}, Failure: &startupFailure{
			Stage:       startupStageStartupSync,
			Cause:       startupErr,
			Diagnostic:  continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: startupErr.Error()}, dbPath),
			Recoverable: true,
		}}
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
		return startupResult{Failure: &startupFailure{Stage: startupStageConfigLoad, Cause: policyErr, Diagnostic: compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: "injected startup failure"}}}
	})
	root.SetArgs([]string{"config", "--json"})
	err := root.Execute()
	if !errors.Is(err, policyErr) {
		t.Fatalf("error=%v, want injected startup failure", err)
	}
	if stdout.Len() == 0 {
		t.Fatalf("startup failure did not emit machine diagnostic")
	}
	if strings.Contains(stdout.String(), "database") || strings.Contains(stdout.String(), "session_dirs") {
		t.Fatalf("handler emitted config payload after startup failure: %q", stdout.String())
	}
}

func TestRecoverBlocksNonrecoverableStartupFailures(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	for _, tc := range []struct {
		name  string
		stage startupStage
	}{
		{name: "input_dir", stage: startupStageInputDir},
		{name: "legacy_source", stage: startupStageLegacySource},
		{name: "config_load", stage: startupStageConfigLoad},
		{name: "active_manifest", stage: startupStageActiveManifest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cause := errors.New("nonrecoverable " + tc.name)
			called := false
			originalExecute := recoverExecute
			recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
				called = true
				return recovery.Report{}, nil
			}
			t.Cleanup(func() { recoverExecute = originalExecute })

			var stdout, stderr bytes.Buffer
			root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
				return startupResult{Config: cfg, Failure: &startupFailure{
					Stage:       tc.stage,
					Cause:       cause,
					Diagnostic:  compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: "blocked " + tc.name},
					Recoverable: false,
				}}
			})
			root.SetArgs([]string{"recover", "--from", "stranded.db", "--dry-run"})
			err := root.Execute()
			if err == nil {
				t.Fatalf("recover unexpectedly succeeded for %s; stdout=%q stderr=%q", tc.stage, stdout.String(), stderr.String())
			}
			if !errors.Is(err, cause) {
				t.Fatalf("error=%v does not preserve startup cause %v", err, cause)
			}
			if called {
				t.Fatal("recoverExecute was called for nonrecoverable startup failure")
			}
			if stdout.Len() != 0 {
				t.Fatalf("nonrecoverable recover emitted stdout: %q", stdout.String())
			}
		})
	}
}

func TestRecoverableStartupFailuresPermitControlledRecovery(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	for _, tc := range []struct {
		name  string
		stage startupStage
	}{
		{name: "index_prepare", stage: startupStageIndexPrepare},
		{name: "startup_sync", stage: startupStageStartupSync},
	} {
		t.Run(tc.name, func(t *testing.T) {
			startupDiag := continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: "recoverable " + tc.name}, cfg.DatabasePath)
			called := false
			originalExecute := recoverExecute
			recoverExecute = func(_ context.Context, opts recovery.Options) (recovery.Report, error) {
				called = true
				if !opts.DryRun || opts.FromPath != cfg.DatabasePath || opts.ActivePath != cfg.DatabasePath {
					t.Fatalf("recovery options=%+v, want dry-run from/active %q", opts, cfg.DatabasePath)
				}
				return recovery.Report{ActivePath: opts.ActivePath}, nil
			}
			t.Cleanup(func() { recoverExecute = originalExecute })

			var stdout, stderr bytes.Buffer
			root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
				return startupResult{Config: cfg, Failure: &startupFailure{Stage: tc.stage, Diagnostic: startupDiag, Recoverable: true}}
			})
			root.SetArgs(startupDiag.Continuation)
			if err := root.Execute(); err != nil {
				t.Fatalf("recoverable %s did not permit dry-run recovery: %v\nstdout=%q stderr=%q", tc.stage, err, stdout.String(), stderr.String())
			}
			if !called {
				t.Fatal("recoverExecute was not called for recoverable startup failure")
			}
			if !strings.Contains(stdout.String(), "recovery dry run") {
				t.Fatalf("dry-run report missing: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestSuccessfulStartupRecoveryFailureOmitsTypedNilStartupFailure(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	recoveryErr := errors.New("injected recovery failure")

	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		return recovery.Report{}, recoveryErr
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err := root.Execute()
	if !errors.Is(err, recoveryErr) {
		t.Fatalf("error=%v does not preserve recovery failure", err)
	}
	if strings.Contains(err.Error(), "startup failure") {
		t.Fatalf("error=%v includes phantom startup failure", err)
	}
	var failure *startupFailure
	if errors.As(err, &failure) {
		t.Fatalf("error=%v unexpectedly matches startupFailure target %#v", err, failure)
	}
}

func TestSuccessfulStartupPostInstallSyncFailureOmitsTypedNilStartupFailure(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	syncErr := errors.New("injected post-install sync failure")

	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		return recovery.Report{ActivePath: cfg.DatabasePath}, nil
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	originalPostInstallSync := recoverPostInstallSync
	recoverPostInstallSync = func(*config.Config, io.Writer) error { return syncErr }
	t.Cleanup(func() { recoverPostInstallSync = originalPostInstallSync })

	var stdout bytes.Buffer
	root := buildRootCmdWithStartup(&stdout, io.Discard, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err := root.Execute()
	if !errors.Is(err, syncErr) {
		t.Fatalf("error=%v does not preserve post-install sync failure", err)
	}
	if strings.Contains(err.Error(), "startup failure") {
		t.Fatalf("error=%v includes phantom startup failure", err)
	}
	var failure *startupFailure
	if errors.As(err, &failure) {
		t.Fatalf("error=%v unexpectedly matches startupFailure target %#v", err, failure)
	}
	if stdout.Len() != 0 {
		t.Fatalf("report printed before failed post-install sync: %q", stdout.String())
	}
}

func TestDiagnosticOnlyStartupFailurePlusRecoveryFailurePreservesBothCauses(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	startupDiag := continuationFor(compat.Diagnostic{Code: compat.CodeUnsupportedLineage, Summary: "diagnostic-only startup"}, cfg.DatabasePath)
	recoveryErr := errors.New("injected recovery failure")

	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		return recovery.Report{}, recoveryErr
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	root := buildRootCmdWithStartup(io.Discard, io.Discard, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg, Failure: &startupFailure{Stage: startupStageIndexPrepare, Diagnostic: startupDiag, Recoverable: true}}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err := root.Execute()
	if !errors.Is(err, recoveryErr) {
		t.Fatalf("error=%v does not preserve recovery failure", err)
	}
	var failure *startupFailure
	if !errors.As(err, &failure) || failure == nil {
		t.Fatalf("error=%v does not expose structural startup failure", err)
	}
	assertDiagnosticAggregateRenderedOnce(t, err, startupDiag, recoveryErr)
}

func TestDiagnosticOnlyStartupFailurePlusPostInstallSyncFailurePreservesBothCauses(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	startupDiag := continuationFor(compat.Diagnostic{Code: compat.CodeUnsupportedLineage, Summary: "diagnostic-only startup"}, cfg.DatabasePath)
	syncErr := errors.New("injected post-install sync failure")

	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		return recovery.Report{ActivePath: cfg.DatabasePath}, nil
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	originalPostInstallSync := recoverPostInstallSync
	recoverPostInstallSync = func(*config.Config, io.Writer) error { return syncErr }
	t.Cleanup(func() { recoverPostInstallSync = originalPostInstallSync })

	var stdout bytes.Buffer
	root := buildRootCmdWithStartup(&stdout, io.Discard, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg, Failure: &startupFailure{Stage: startupStageIndexPrepare, Diagnostic: startupDiag, Recoverable: true}}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err := root.Execute()
	if !errors.Is(err, syncErr) {
		t.Fatalf("error=%v does not preserve post-install sync failure", err)
	}
	var failure *startupFailure
	if !errors.As(err, &failure) || failure == nil {
		t.Fatalf("error=%v does not expose structural startup failure", err)
	}
	assertDiagnosticAggregateRenderedOnce(t, err, startupDiag, syncErr)
	if stdout.Len() != 0 {
		t.Fatalf("report printed before failed post-install sync: %q", stdout.String())
	}
}

func assertDiagnosticAggregateRenderedOnce(t *testing.T, err error, diagnostic compat.Diagnostic, cause error) {
	t.Helper()
	if err == nil {
		t.Fatal("error is nil")
	}
	text := err.Error()
	for _, want := range []string{string(diagnostic.Code), diagnostic.Summary, strings.Join(diagnostic.Continuation, " "), cause.Error()} {
		if got := strings.Count(text, want); got != 1 {
			t.Fatalf("error=%q contains %q %d times, want exactly once", text, want, got)
		}
	}
}

func TestStartupFailureNilAndFallbackRendering(t *testing.T) {
	var nilFailure *startupFailure
	if got := nilFailure.Error(); got != "startup failure" {
		t.Fatalf("nil startup failure Error() = %q, want startup failure", got)
	}
	if got := nilFailure.Unwrap(); got != nil {
		t.Fatalf("nil startup failure Unwrap() = %v, want nil", got)
	}

	fallback := (&startupFailure{}).Error()
	for _, want := range []string{string(startupStageUnknown), "startup failed"} {
		if !strings.Contains(fallback, want) {
			t.Fatalf("fallback startup failure rendering = %q, want %q", fallback, want)
		}
	}
}

func TestStartupFailureMachineDiagnosticsAreStructuredAndUncontaminated(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		mode string
	}{
		{name: "config_json", argv: []string{"config", "--json"}, mode: "json"},
		{name: "manifest_robot", argv: []string{"search", "needle", "--robot"}, mode: "robot"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
				return startupResult{Failure: &startupFailure{
					Stage:      startupStageActiveManifest,
					Cause:      errors.New("active manifest invalid"),
					Diagnostic: compat.Diagnostic{Code: compat.CodeMigrationFailed, Summary: "active manifest invalid"},
				}}
			})
			root.SetArgs(tc.argv)
			err := root.Execute()
			if err == nil {
				t.Fatalf("%v unexpectedly succeeded", tc.argv)
			}
			if stderr.Len() != 0 {
				t.Fatalf("machine diagnostic contaminated stderr: %q", stderr.String())
			}
			if strings.Contains(stdout.String(), "source_path") || strings.Contains(stdout.String(), "database") || strings.Contains(stdout.String(), "Error:") {
				t.Fatalf("machine diagnostic contaminated stdout: %q", stdout.String())
			}
			switch tc.mode {
			case "json":
				var got struct {
					Code         string   `json:"code"`
					Summary      string   `json:"summary"`
					Continuation []string `json:"continuation_argv"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
					t.Fatalf("invalid JSON diagnostic %q: %v", stdout.String(), err)
				}
				if got.Code == "" || got.Summary == "" || len(got.Continuation) != 0 {
					t.Fatalf("JSON diagnostic=%+v, want code/summary and no continuation", got)
				}
			case "robot":
				fields := robotDiagnosticFields(t, stdout.String())
				if fields["diagnostic_code"] == "" || fields["diagnostic_summary"] == "" {
					t.Fatalf("robot diagnostic missing code/summary: %q", stdout.String())
				}
				if _, ok := fields["diagnostic_continuation_argv"]; ok {
					t.Fatalf("nonrecoverable robot diagnostic included continuation: %q", stdout.String())
				}
			}
		})
	}
}

func TestRobotDiagnosticEscapesMultilineValuesAndEncodesContinuationArgv(t *testing.T) {
	var stdout, stderr bytes.Buffer
	diag := compat.Diagnostic{
		Code:         compat.CodeIndexStale,
		Summary:      "first line\\with slash\r\nsecond line",
		Continuation: []string{"recover", "--from", "path with spaces\\and\\slashes\r\nnext", "--dry-run"},
	}
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Failure: &startupFailure{Stage: startupStageStartupSync, Diagnostic: diag, Recoverable: true}}
	})
	root.SetArgs([]string{"search", "needle", "--robot"})
	if err := root.Execute(); err == nil {
		t.Fatalf("robot diagnostic command unexpectedly succeeded; stdout=%q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("robot diagnostic wrote stderr: %q", stderr.String())
	}
	fields := robotDiagnosticFields(t, stdout.String())
	if got := fields["diagnostic_summary"]; strings.ContainsAny(got, "\r\n") || !strings.Contains(got, `\\`) || !strings.Contains(got, `\r`) || !strings.Contains(got, `\n`) {
		t.Fatalf("robot summary was not escaped as one line: %q in output %q", got, stdout.String())
	}
	continuation := fields["diagnostic_continuation_argv"]
	if continuation == "" || !strings.HasPrefix(continuation, "[") {
		t.Fatalf("robot continuation is not encoded argv JSON: %q", continuation)
	}
	if strings.ContainsAny(continuation, "\r\n") || !strings.Contains(continuation, `\\`) || !strings.Contains(continuation, `\r`) || !strings.Contains(continuation, `\n`) {
		t.Fatalf("robot continuation was not escaped as one line: %q", continuation)
	}
}

func robotDiagnosticFields(t *testing.T, output string) map[string]string {
	t.Helper()
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			t.Fatalf("robot line is not key=value: %q in %q", line, output)
		}
		if strings.ContainsAny(parts[1], "\r\n") {
			t.Fatalf("robot value contains raw newline: %q in %q", parts[1], output)
		}
		fields[parts[0]] = parts[1]
	}
	return fields
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
	if result.Failure != nil {
		t.Fatalf("startup result=%+v", result)
	}
	if calls != 1 {
		t.Fatalf("sync calls=%d, want 1", calls)
	}
}

func TestDefaultStartupPolicyNonrecoverableStages(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage startupStage
		setup func(t *testing.T)
	}{
		{
			name:  "legacy_source",
			stage: startupStageLegacySource,
			setup: func(t *testing.T) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				configDir := filepath.Join(home, ".config", "backscroll")
				if err := os.MkdirAll(configDir, 0o755); err != nil {
					t.Fatalf("mkdir config dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte("[sources]\nke = [\"legacy.md\"]\n"), 0o644); err != nil {
					t.Fatalf("write legacy config: %v", err)
				}
			},
		},
		{
			name:  "config_load",
			stage: startupStageConfigLoad,
			setup: func(t *testing.T) {
				t.Helper()
				dir := t.TempDir()
				t.Setenv("HOME", filepath.Join(dir, "home"))
				t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
				t.Chdir(dir)
				if err := os.WriteFile(filepath.Join(dir, "backscroll.toml"), []byte("database_path = [\n"), 0o644); err != nil {
					t.Fatalf("write malformed local config: %v", err)
				}
			},
		},
		{
			name:  "active_manifest",
			stage: startupStageActiveManifest,
			setup: func(t *testing.T) {
				t.Helper()
				dir := t.TempDir()
				cfgDir := filepath.Join(dir, "config")
				setIndexPolicyEnv(t, filepath.Join(dir, "index.db"), cfgDir)
				inputsDir := filepath.Join(cfgDir, "backscroll", "inputs")
				if err := os.MkdirAll(inputsDir, 0o755); err != nil {
					t.Fatalf("mkdir inputs dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(inputsDir, "bad.inputs.toml"), []byte("[[inputs]\nid = [\n"), 0o644); err != nil {
					t.Fatalf("write malformed manifest: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalSync := startupSync
			startupSync = func(*config.Config, io.Writer) error {
				t.Fatal("startup sync should not run after nonrecoverable startup stage")
				return nil
			}
			t.Cleanup(func() { startupSync = originalSync })
			tc.setup(t)

			result := defaultStartupPolicy(context.Background(), io.Discard)
			failure := result.startupFailure()
			if failure == nil {
				t.Fatal("default startup unexpectedly succeeded")
			}
			if failure.Stage != tc.stage || failure.Recoverable {
				t.Fatalf("failure=%+v, want stage %s and nonrecoverable", failure, tc.stage)
			}
			if failure.Cause == nil || failure.Diagnostic.Code == "" || failure.Diagnostic.Summary == "" || len(failure.Diagnostic.Continuation) != 0 {
				t.Fatalf("failure diagnostic/cause not structured as nonrecoverable: %+v", failure)
			}
		})
	}
}

func TestDiagnosticAlreadyRenderedOnlySuppressesTopLevelDiagnostic(t *testing.T) {
	diagErr := indexDiagnosticError{diagnostic: compat.Diagnostic{Code: compat.CodeIndexStale, Summary: "already rendered"}}
	if !diagnosticAlreadyRendered(diagErr) {
		t.Fatal("top-level rendered diagnostic should be suppressed by main")
	}
	joined := errors.Join(diagErr, errors.New("recovery failed"))
	if diagnosticAlreadyRendered(joined) {
		t.Fatal("joined recovery failure should still be printed by main")
	}
	if diagErr.Error() == "" {
		t.Fatal("diagnostic error should render a non-empty message")
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
	gotFailure := got.startupFailure()
	wantFailure := want.startupFailure()
	if (gotFailure == nil) != (wantFailure == nil) {
		t.Fatalf("startup failure nil mismatch: got=%+v want=%+v", gotFailure, wantFailure)
	}
	if gotFailure == nil {
		return
	}
	if gotFailure.Stage != wantFailure.Stage || gotFailure.Recoverable != wantFailure.Recoverable {
		t.Fatalf("startup failure metadata=%+v, want %+v", gotFailure, wantFailure)
	}
	if (gotFailure.Cause == nil) != (wantFailure.Cause == nil) {
		t.Fatalf("startup cause nil mismatch: got=%v want=%v", gotFailure.Cause, wantFailure.Cause)
	}
	if gotFailure.Cause != nil && gotFailure.Cause.Error() != wantFailure.Cause.Error() {
		t.Fatalf("startup cause=%q, want %q", gotFailure.Cause.Error(), wantFailure.Cause.Error())
	}
	if gotFailure.Diagnostic.Code != wantFailure.Diagnostic.Code || gotFailure.Diagnostic.Summary != wantFailure.Diagnostic.Summary || !reflect.DeepEqual(gotFailure.Diagnostic.Continuation, wantFailure.Diagnostic.Continuation) {
		t.Fatalf("startup diagnostic=%+v, want %+v", gotFailure.Diagnostic, wantFailure.Diagnostic)
	}
}
