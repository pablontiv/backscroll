package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/recovery"
	"github.com/pablontiv/backscroll/internal/storage"
)

type fileSnapshot struct {
	Bytes []byte
	Mode  os.FileMode
	MTime time.Time
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
	root := buildRootCmd(&stdout, &stderr)
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
	root := buildRootCmd(&stdout, &stderr)
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
	root := buildRootCmd(&stdout, &stderr)
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
	root := buildRootCmd(&stdout, &stderr)
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
	root := buildRootCmd(&stdout, &stderr)
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
	root := buildRootCmd(&stdout, &stderr)
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
	cmd := buildRootCmd(&stdout, &stderr)
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
			cmd := buildRootCmd(&stdout, &stderr)
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
