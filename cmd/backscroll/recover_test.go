package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/recovery"
	"github.com/pablontiv/backscroll/internal/storage"
	"github.com/spf13/cobra"
)

type fileSnapshot struct {
	Bytes []byte
	Mode  os.FileMode
	MTime time.Time
}

type firstWriteMarker struct {
	bytes.Buffer
	events *[]string
	marked bool
}

func (w *firstWriteMarker) Write(p []byte) (int, error) {
	if !w.marked {
		*w.events = append(*w.events, "report")
		w.marked = true
	}
	return w.Buffer.Write(p)
}

func buildRecoverRootWithConfig(t *testing.T, stdout, stderr io.Writer, activePath string) *cobra.Command {
	t.Helper()
	emptyInputs := filepath.Join(t.TempDir(), "empty-inputs")
	if err := os.MkdirAll(emptyInputs, 0o755); err != nil {
		t.Fatalf("mkdir empty recovery inputs: %v", err)
	}
	cfg := &config.Config{DatabasePath: activePath, SessionDirs: []string{emptyInputs}}
	return buildRootCmdWithStartup(stdout, stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg}
	})
}

func TestRecoverExecuteReceivesCommandContext(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	type contextKey string
	const markerKey contextKey = "recover-context-marker"
	baseCtx := context.WithValue(context.Background(), markerKey, "present")
	ctx, cancel := context.WithCancel(baseCtx)
	cancel()

	called := false
	originalExecute := recoverExecute
	recoverExecute = func(execCtx context.Context, opts recovery.Options) (recovery.Report, error) {
		called = true
		if got := execCtx.Value(markerKey); got != "present" {
			t.Fatalf("recovery context marker = %v, want present", got)
		}
		if !errors.Is(execCtx.Err(), context.Canceled) {
			t.Fatalf("recovery context err = %v, want context canceled", execCtx.Err())
		}
		if opts.ActivePath != cfg.DatabasePath || opts.FromPath != "stranded.db" || !opts.DryRun {
			t.Fatalf("recovery options = %+v, want active=%q from=stranded.db dryRun=true", opts, cfg.DatabasePath)
		}
		return recovery.Report{ActivePath: opts.ActivePath}, nil
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	var stdout, stderr bytes.Buffer
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg}
	})
	root.SetContext(ctx)
	root.SetArgs([]string{"recover", "--from", "stranded.db", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("recover returned error: %v", err)
	}
	if !called {
		t.Fatal("recover execute seam was not called")
	}
}

func TestRecoverPostInstallSyncBeforeReport(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	events := []string{}
	stdout := &firstWriteMarker{events: &events}
	var stderr bytes.Buffer

	originalExecute := recoverExecute
	recoverExecute = func(_ context.Context, opts recovery.Options) (recovery.Report, error) {
		events = append(events, "recover")
		if opts.ActivePath != cfg.DatabasePath || opts.FromPath != "stranded.db" || opts.DryRun {
			t.Fatalf("recovery options = %+v, want active=%q from=stranded.db dryRun=false", opts, cfg.DatabasePath)
		}
		return recovery.Report{ActivePath: opts.ActivePath}, nil
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	originalPostInstallSync := recoverPostInstallSync
	recoverPostInstallSync = func(got *config.Config, progress io.Writer) error {
		events = append(events, "sync")
		if got != cfg {
			t.Fatalf("post-install sync config pointer = %p, want %p", got, cfg)
		}
		if progress != &stderr {
			t.Fatalf("post-install sync progress writer = %T, want stderr buffer", progress)
		}
		return nil
	}
	t.Cleanup(func() { recoverPostInstallSync = originalPostInstallSync })

	root := buildRootCmdWithStartup(stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	if err := root.Execute(); err != nil {
		t.Fatalf("recover returned error: %v", err)
	}
	want := []string{"recover", "sync", "report"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v, want %v", events, want)
	}
}

func TestRecoverDryRunSkipsPostInstallSync(t *testing.T) {
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}
	originalExecute := recoverExecute
	recoverExecute = func(_ context.Context, opts recovery.Options) (recovery.Report, error) {
		if !opts.DryRun {
			t.Fatalf("DryRun = false, want true")
		}
		return recovery.Report{ActivePath: opts.ActivePath}, nil
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	syncCalled := false
	originalPostInstallSync := recoverPostInstallSync
	recoverPostInstallSync = func(*config.Config, io.Writer) error {
		syncCalled = true
		return nil
	}
	t.Cleanup(func() { recoverPostInstallSync = originalPostInstallSync })

	var stdout, stderr bytes.Buffer
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatalf("recover dry-run returned error: %v", err)
	}
	if syncCalled {
		t.Fatal("post-install sync ran during dry-run")
	}
}

func TestRecoverPostInstallSyncFailurePreservesStartupCause(t *testing.T) {
	startupErr := errors.New("injected startup failure")
	syncErr := errors.New("injected post-sync failure")
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}

	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		return recovery.Report{ActivePath: cfg.DatabasePath}, nil
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	originalPostInstallSync := recoverPostInstallSync
	recoverPostInstallSync = func(*config.Config, io.Writer) error {
		return syncErr
	}
	t.Cleanup(func() { recoverPostInstallSync = originalPostInstallSync })

	var stdout, stderr bytes.Buffer
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg, Failure: &startupFailure{
			Stage:       startupStageStartupSync,
			Cause:       startupErr,
			Diagnostic:  continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: startupErr.Error()}, cfg.DatabasePath),
			Recoverable: true,
		}}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	err := root.Execute()
	if !errors.Is(err, startupErr) {
		t.Fatalf("error=%v does not preserve startup failure", err)
	}
	if !errors.Is(err, syncErr) {
		t.Fatalf("error=%v does not preserve post-sync failure", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("report printed before failed post-sync: %q", stdout.String())
	}
}

func TestRecoverSuccessfulContinuationRemediatesStartupFailure(t *testing.T) {
	startupErr := errors.New("injected startup failure")
	cfg := &config.Config{DatabasePath: filepath.Join(t.TempDir(), "active.db")}

	originalExecute := recoverExecute
	recoverExecute = func(context.Context, recovery.Options) (recovery.Report, error) {
		return recovery.Report{ActivePath: cfg.DatabasePath}, nil
	}
	t.Cleanup(func() { recoverExecute = originalExecute })

	originalPostInstallSync := recoverPostInstallSync
	recoverPostInstallSync = func(*config.Config, io.Writer) error { return nil }
	t.Cleanup(func() { recoverPostInstallSync = originalPostInstallSync })

	var stdout, stderr bytes.Buffer
	root := buildRootCmdWithStartup(&stdout, &stderr, func(context.Context, io.Writer) startupResult {
		return startupResult{Config: cfg, Failure: &startupFailure{
			Stage:       startupStageStartupSync,
			Cause:       startupErr,
			Diagnostic:  continuationFor(compat.Diagnostic{Code: compat.CodeIndexStale, Summary: startupErr.Error()}, cfg.DatabasePath),
			Recoverable: true,
		}}
	})
	root.SetArgs([]string{"recover", "--from", "stranded.db"})
	if err := root.Execute(); err != nil {
		t.Fatalf("recover returned startup failure after successful remediation: %v", err)
	}
}

func TestRecoverDryRunMatchesUnionApplyPlanWithoutWrites(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create isolated HOME: %v", err)
	}
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoverTestDB(t, activePath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "kept active row",
		UUID:        "11111111-1111-4111-8111-111111111111",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})
	createRecoverTestDB(t, fromPath, []storage.IndexedMessage{
		{
			Ordinal:     0,
			Role:        "user",
			Text:        "kept active row",
			UUID:        "11111111-1111-4111-8111-111111111111",
			Timestamp:   "2026-08-18T00:00:00Z",
			ContentType: "text",
		},
		{
			Ordinal:     1,
			Role:        "assistant",
			Text:        "rescued stranded row",
			UUID:        "22222222-2222-4222-8222-222222222222",
			Timestamp:   "2026-08-18T00:01:00Z",
			ContentType: "text",
		},
	})

	expected := expectedRecoveryPlan(t, activePath, fromPath)
	if len(expected.InputShapes) != 2 {
		t.Fatalf("expected recovery input shapes = %d, want 2", len(expected.InputShapes))
	}
	before := inventoryDirectory(t, dir)

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", activePath)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	root := buildRecoverRootWithConfig(t, &stdout, &stderr, activePath)
	root.SetArgs([]string{"recover", "--from", fromPath, "--dry-run"})
	err := root.Execute()
	if err != nil {
		t.Fatalf("recover dry run returned error: %v\nstderr=%s", err, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	after := inventoryDirectory(t, dir)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("dry run mutated active directory inventory\nbefore: %s\nafter:  %s", describeInventory(before), describeInventory(after))
	}

	resolvedActivePath := canonicalRecoverTestPath(t, activePath)
	out := stdout.String()
	wantParts := []string{
		"recovery dry run",
		"active path: " + resolvedActivePath,
		"replacement target: " + resolvedActivePath,
		"backup path: " + filepath.Join(filepath.Dir(resolvedActivePath), "."+filepath.Base(resolvedActivePath)+".backup-<UTC>-<randomhex>"),
		"input 1 records: 1",
		"input 2 records: 2",
		"exact duplicates: 1",
		"final count: 2",
		fmt.Sprintf("input 1 shape: version=%d signature=%s", expected.InputShapes[0].AppliedVersion, expected.InputShapes[0].Signature),
		fmt.Sprintf("input 2 shape: version=%d signature=%s", expected.InputShapes[1].AppliedVersion, expected.InputShapes[1].Signature),
	}
	for _, want := range wantParts {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q\nstdout:\n%s", want, out)
		}
	}
}

func TestRecoverCommandPreservesApplyFailureAs(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create isolated HOME: %v", err)
	}
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	sharedUUID := "99999999-9999-4999-8999-999999999999"
	createRecoverTestDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "cli active conflict", UUID: sharedUUID, ContentType: "text"}})
	createRecoverTestDB(t, fromPath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "cli stranded conflict", UUID: sharedUUID, ContentType: "text"}})

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", activePath)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	root := buildRecoverRootWithConfig(t, &stdout, &stderr, activePath)
	root.SetArgs([]string{"recover", "--from", fromPath})
	err := root.Execute()
	if err == nil {
		t.Fatal("recover command succeeded; want planning conflict error")
	}
	var failure *recovery.ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("command error %T %[1]v, want errors.As to *recovery.ApplyFailure", err)
	}
	if failure.Phase != recovery.ApplyFailurePhase("planning") {
		t.Fatalf("ApplyFailure phase = %s, want planning", failure.Phase)
	}
}

func TestRecoverApplyReportsActualBackupAndCounts(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create isolated HOME: %v", err)
	}
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoverTestDB(t, activePath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "cli active row",
		UUID:        "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})
	createRecoverTestDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "assistant",
		Text:        "cli stranded row",
		UUID:        "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		Timestamp:   "2026-08-18T00:01:00Z",
		ContentType: "text",
	}})
	activeBefore, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("read active before recovery: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", activePath)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	root := buildRecoverRootWithConfig(t, &stdout, &stderr, activePath)
	root.SetArgs([]string{"recover", "--from", fromPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("recover apply returned error: %v\nstderr=%s", err, stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	resolvedActivePath := canonicalRecoverTestPath(t, activePath)
	out := stdout.String()
	backupPath := recoverOutputValue(t, out, "backup path: ")
	wantParts := []string{
		"recovery complete",
		"active path: " + resolvedActivePath,
		"replacement target: " + resolvedActivePath,
		"backup path: " + backupPath,
		"input 1 records: 1",
		"input 2 records: 1",
		"exact duplicates: 0",
		"final count: 2",
	}
	for _, want := range wantParts {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q\nstdout:\n%s", want, out)
		}
	}
	backupBytes, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read reported backup %s: %v", backupPath, err)
	}
	if !bytes.Equal(backupBytes, activeBefore) {
		t.Fatalf("reported backup bytes differ from original active bytes")
	}
}

func TestRecoverInstalledDestinationPassesIndexedValidation(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	activePath := filepath.Join(dir, "active.db")
	strandedPath := filepath.Join(dir, "stranded.db")
	createRecoverFixtureDB(t, activePath, "active-v13.sql", map[string]string{
		"uuid-active-v13": "11111111-1111-4111-8111-111111111111",
	})
	createRecoverFixtureDB(t, strandedPath, "stranded-v7.sql", map[string]string{
		"uuid-stranded-v7": "22222222-2222-4222-8222-222222222222",
	})
	activeBefore, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", activePath)
	t.Chdir(dir)

	var recoverOut, recoverErr bytes.Buffer
	recoverCmd := buildRecoverRootWithConfig(t, &recoverOut, &recoverErr, activePath)
	recoverCmd.SetArgs([]string{"recover", "--from", strandedPath})
	if err := recoverCmd.Execute(); err != nil {
		t.Fatalf("recover: %v\nstderr=%s", err, recoverErr.String())
	}
	if recoverErr.String() != "" {
		t.Fatalf("recover stderr = %q, want empty", recoverErr.String())
	}
	backupPath := recoverOutputValue(t, recoverOut.String(), "backup path: ")
	backupBytes, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(backupBytes, activeBefore) {
		t.Fatal("active backup differs from pre-recovery database")
	}

	var validateOut, validateErr bytes.Buffer
	validateCmd := buildRootCmd(&validateOut, &validateErr)
	validateCmd.SetArgs([]string{"validate"})
	if err := validateCmd.Execute(); err != nil {
		t.Fatalf("validate recovered destination: %v\nstdout=%s\nstderr=%s", err, validateOut.String(), validateErr.String())
	}
	if validateErr.String() != "" {
		t.Fatalf("validate stderr = %q, want empty", validateErr.String())
	}

	db, err := storage.OpenReadOnly(activePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var records, accountedPaths, ftsHits int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM search_items`).Scan(&records); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM indexed_files`).Scan(&accountedPaths); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'default'`).Scan(&ftsHits); err != nil {
		t.Fatal(err)
	}
	if records != 4 || accountedPaths != 4 || ftsHits != 2 {
		t.Fatalf("recovered database counts records=%d paths=%d fts=%d, want 4/4/2", records, accountedPaths, ftsHits)
	}
}

func TestRecoverCommandReturnsStructuredApplyFailure(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create isolated HOME: %v", err)
	}
	missingActivePath := filepath.Join(dir, "missing-active.db")
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoverTestDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "assistant",
		Text:        "cli stranded structured failure",
		UUID:        "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
		ContentType: "text",
	}})

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", missingActivePath)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	root := buildRecoverRootWithConfig(t, &stdout, &stderr, missingActivePath)
	root.SetArgs([]string{"recover", "--from", fromPath})
	err := root.Execute()
	if err == nil {
		t.Fatalf("recover apply succeeded; want structured failure")
	}
	var failure *recovery.ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T %[1]v, want *recovery.ApplyFailure", err)
	}
	if failure.ActivePath == "" || failure.State == "" || failure.Phase == "" {
		t.Fatalf("incomplete structured failure: %+v", failure)
	}
}

func TestRecoverCommandEmptyFromReturnsApplyFailure(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create isolated HOME: %v", err)
	}
	activePath := filepath.Join(dir, "active.db")

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", activePath)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	root := buildRecoverRootWithConfig(t, &stdout, &stderr, activePath)
	root.SetArgs([]string{"recover", "--from", ""})
	err := root.Execute()
	if err == nil {
		t.Fatal("recover command succeeded; want structured missing --from failure")
	}
	var failure *recovery.ApplyFailure
	if !errors.As(err, &failure) {
		t.Fatalf("command error %T %[1]v, want *recovery.ApplyFailure", err)
	}
	if failure.Phase != recovery.ApplyFailurePhase("source-read") || failure.ActivePath == "" || failure.FromPath != "" {
		t.Fatalf("ApplyFailure = %+v, want source-read with active and missing from", failure)
	}
}

func TestRecoverCommandPathCanonicalizationFailurePreservesApplyFailureAs(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("create isolated HOME: %v", err)
	}
	activePath := filepath.Join(dir, "active.db")
	fromPath := filepath.Join(dir, "broken-link.db")
	createRecoverTestDB(t, activePath, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "cli active", UUID: "dddddddd-dddd-4ddd-8ddd-dddddddddddd", ContentType: "text"}})
	if err := os.Symlink(filepath.Join(dir, "missing-target.db"), fromPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("BACKSCROLL_CONFIG_DIR", filepath.Join(dir, "config"))
	t.Setenv("BACKSCROLL_DATABASE_PATH", activePath)
	t.Chdir(dir)

	var stdout, stderr bytes.Buffer
	root := buildRecoverRootWithConfig(t, &stdout, &stderr, activePath)
	root.SetArgs([]string{"recover", "--from", fromPath})
	execErr := root.Execute()
	if execErr == nil {
		t.Fatal("recover command succeeded; want structured symlink failure")
	}
	var failure *recovery.ApplyFailure
	if !errors.As(execErr, &failure) {
		t.Fatalf("command error %T %[1]v, want *recovery.ApplyFailure", execErr)
	}
	wantFromPath, err := filepath.Abs(fromPath)
	if err != nil {
		t.Fatalf("absolute from path: %v", err)
	}
	if failure.FromPath != wantFromPath || failure.ActivePath != canonicalRecoverTestPath(t, activePath) {
		t.Fatalf("ApplyFailure paths active=%q from=%q", failure.ActivePath, failure.FromPath)
	}
	if !strings.Contains(execErr.Error(), "resolve database symlink") {
		t.Fatalf("error = %v, want symlink details", execErr)
	}
}

func TestRecoverRejectsMissingFrom(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := buildRecoverRootWithConfig(t, &stdout, &stderr, filepath.Join(t.TempDir(), "active.db"))
	cmd.SetArgs([]string{"recover", "--dry-run"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("recover without --from succeeded; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(err.Error(), `required flag(s) "from" not set`) && !strings.Contains(stderr.String(), `required flag(s) "from" not set`) {
		t.Fatalf("missing --from error = %v, stderr=%q", err, stderr.String())
	}
}

func TestRecoverHelpDocumentsRequiredFromFlagWithoutConfig(t *testing.T) {
	stdout, stderr, err := runCmd("recover", "--help")
	if err != nil {
		t.Fatalf("recover --help failed: %v stderr=%q", err, stderr)
	}
	for _, want := range []string{"Recover stranded database rows", "--from string", "--dry-run"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("recover --help missing %q in:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("recover --help wrote stderr: %q", stderr)
	}
}

func TestRecoverHasNoGeneralMergeFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "into", args: []string{"recover", "--from", "one.db", "--into", "two.db", "--dry-run"}},
		{name: "force", args: []string{"recover", "--from", "one.db", "--force", "--dry-run"}},
		{name: "partial", args: []string{"recover", "--from", "one.db", "--partial", "--dry-run"}},
		{name: "merge", args: []string{"recover", "--from", "one.db", "--merge", "--dry-run"}},
		{name: "skip", args: []string{"recover", "--from", "one.db", "--skip", "--dry-run"}},
		{name: "repeated-from", args: []string{"recover", "--from", "one.db", "--from", "two.db", "--dry-run"}},
		{name: "positional-arg", args: []string{"recover", "--from", "one.db", "--dry-run", "extra.db"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := buildRecoverRootWithConfig(t, &stdout, &stderr, filepath.Join(t.TempDir(), "active.db"))
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("%v succeeded; stdout=%q stderr=%q", tt.args, stdout.String(), stderr.String())
			}
			if stdout.String() != "" {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
		})
	}
}

func createRecoverFixtureDB(t *testing.T, path, fixture string, replacements map[string]string) {
	t.Helper()
	fixturePath := filepath.Join("..", "..", "tests", "fixtures", "recovery", fixture)
	body, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read recovery fixture %s: %v", fixturePath, err)
	}
	sqlText := string(body)
	for from, to := range replacements {
		sqlText = strings.ReplaceAll(sqlText, from, to)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open recovery fixture %s: %v", path, err)
	}
	if _, err := db.Exec(sqlText); err != nil {
		_ = db.Close()
		t.Fatalf("execute recovery fixture %s: %v", fixture, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close recovery fixture %s: %v", path, err)
	}
}

func createRecoverTestDB(t *testing.T, path string, messages []storage.IndexedMessage) {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("open test database %s: %v", path, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close test database %s: %v", path, err)
		}
	}()

	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/sessions/shared.jsonl",
		Source:     "session",
		Hash:       "hash-" + filepath.Base(path),
		Project:    "project",
		Messages:   messages,
	}}); err != nil {
		t.Fatalf("sync test database %s: %v", path, err)
	}
}

func expectedRecoveryPlan(t *testing.T, paths ...string) compat.RecoveryPlan {
	t.Helper()
	inputs := make([]compat.RecoveryInput, 0, len(paths))
	for _, path := range paths {
		db, err := storage.OpenImmutableReadOnly(path)
		if err != nil {
			t.Fatalf("open immutable readonly %s: %v", path, err)
		}
		input, diag, err := storage.ReadRecoveryInput(context.Background(), db)
		closeErr := db.Close()
		if err != nil {
			t.Fatalf("read recovery input %s: %v", path, err)
		}
		if diag != nil {
			t.Fatalf("read recovery input %s diagnostic: %+v", path, diag)
		}
		if closeErr != nil {
			t.Fatalf("close readonly %s: %v", path, closeErr)
		}
		inputs = append(inputs, input)
	}
	plan, diagnostics, err := compat.PlanRecovery(inputs)
	if err != nil {
		t.Fatalf("PlanRecovery: %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("PlanRecovery diagnostics: %+v", diagnostics)
	}
	return plan
}

func canonicalRecoverTestPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("absolute path for %s: %v", path, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatalf("resolve path for %s: %v", abs, err)
	}
	return resolved
}

func inventoryDirectory(t *testing.T, root string) map[string]fileSnapshot {
	t.Helper()
	inventory := map[string]fileSnapshot{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		inventory[rel] = fileSnapshot{Bytes: bytes, Mode: info.Mode(), MTime: info.ModTime()}
		return nil
	}); err != nil {
		t.Fatalf("inventory %s: %v", root, err)
	}
	return inventory
}

func recoverOutputValue(t *testing.T, output, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if value == "" {
				t.Fatalf("output line %q has empty value", line)
			}
			return value
		}
	}
	t.Fatalf("output missing prefix %q\noutput:\n%s", prefix, output)
	return ""
}

func describeInventory(inventory map[string]fileSnapshot) string {
	paths := make([]string, 0, len(inventory))
	for path := range inventory {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	for _, path := range paths {
		entry := inventory[path]
		parts = append(parts, fmt.Sprintf("%s bytes=%d mode=%s mtime=%s", path, len(entry.Bytes), entry.Mode, entry.MTime.Format(time.RFC3339Nano)))
	}
	return strings.Join(parts, "; ")
}
