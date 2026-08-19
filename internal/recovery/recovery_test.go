package recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestRecoverSameResolvedPathIsOneInput(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	createRecoveryDB(t, activePath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "same database row",
		UUID:        "11111111-1111-4111-8111-111111111111",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})

	aliasPath := filepath.Join(dir, "alias.db")
	if err := os.Symlink(activePath, aliasPath); err != nil {
		t.Fatalf("symlink active database: %v", err)
	}

	report, err := Execute(context.Background(), Options{
		ActivePath: activePath,
		FromPath:   aliasPath,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("Execute dry run: %v", err)
	}

	if !reflect.DeepEqual(report.InputCounts, []int{1}) {
		t.Fatalf("InputCounts = %v, want one active input with one row", report.InputCounts)
	}
	if len(report.Shapes) != 1 {
		t.Fatalf("Shapes = %d, want 1", len(report.Shapes))
	}
	if report.FinalCount != 1 || report.ExactDuplicates != 0 {
		t.Fatalf("FinalCount=%d ExactDuplicates=%d, want 1 and 0", report.FinalCount, report.ExactDuplicates)
	}
}

func TestExecuteWrapsImmutableLiveWALErrorWithRecoveryGuidance(t *testing.T) {
	dir := t.TempDir()
	activePath := filepath.Join(dir, "active.db")
	active := createRecoveryLiveWALDB(t, activePath)
	defer func() { _ = active.Close() }()
	fromPath := filepath.Join(dir, "stranded.db")
	createRecoveryDB(t, fromPath, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "assistant",
		Text:        "stranded row",
		UUID:        "22222222-2222-4222-8222-222222222222",
		Timestamp:   "2026-08-18T00:01:00Z",
		ContentType: "text",
	}})

	_, err := Execute(context.Background(), Options{ActivePath: activePath, FromPath: fromPath, DryRun: true})
	if err == nil {
		t.Fatal("Execute with active live WAL succeeded; want immutable recovery planning guidance")
	}
	if !errors.Is(err, storage.ErrImmutableReadOnlyWALUnsafe) {
		t.Fatalf("Execute error = %v, want ErrImmutableReadOnlyWALUnsafe", err)
	}
	message := err.Error()
	resolvedActive, resolveErr := filepath.EvalSymlinks(activePath)
	if resolveErr != nil {
		t.Fatalf("resolve active path: %v", resolveErr)
	}
	for _, want := range []string{"recovery dry-run", "checkpoint", resolvedActive} {
		if !strings.Contains(message, want) {
			t.Fatalf("Execute error %q missing %q", message, want)
		}
	}
}

func TestResolvePathSurfacesStatErrors(t *testing.T) {
	dir := t.TempDir()
	loop := filepath.Join(dir, "loop.db")
	if err := os.Symlink(loop, loop); err != nil {
		t.Fatalf("create symlink loop: %v", err)
	}

	_, err := resolvePath(loop)
	if err == nil {
		t.Fatal("resolvePath symlink loop succeeded; want stat error")
	}
	if os.IsNotExist(err) {
		t.Fatalf("resolvePath error = %v, want non-NotExist stat error", err)
	}
	if !strings.Contains(err.Error(), "stat database path") {
		t.Fatalf("resolvePath error = %v, want stat database path context", err)
	}
}

func TestResolvePathAllowsMissingPathForOpenError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	resolved, err := resolvePath(missing)
	if err != nil {
		t.Fatalf("resolvePath missing path: %v", err)
	}
	if resolved.path == "" || resolved.info != nil {
		t.Fatalf("resolvePath missing = %+v, want path with nil info", resolved)
	}
}

func createRecoveryDB(t *testing.T, path string, messages []storage.IndexedMessage) {
	t.Helper()
	db := openRecoveryDB(t, path, messages)
	if err := db.Close(); err != nil {
		t.Fatalf("close test database %s: %v", path, err)
	}
}

func createRecoveryLiveWALDB(t *testing.T, path string) *storage.Database {
	t.Helper()
	db := openRecoveryDB(t, path, []storage.IndexedMessage{{
		Ordinal:     0,
		Role:        "user",
		Text:        "active live WAL row",
		UUID:        "11111111-1111-4111-8111-111111111111",
		Timestamp:   "2026-08-18T00:00:00Z",
		ContentType: "text",
	}})
	wal, err := os.Stat(path + "-wal")
	if err != nil {
		_ = db.Close()
		t.Fatalf("stat live WAL sidecar: %v", err)
	}
	if wal.Size() == 0 {
		_ = db.Close()
		t.Fatal("live WAL sidecar is empty; test fixture did not create committed WAL frames")
	}
	return db
}

func openRecoveryDB(t *testing.T, path string, messages []storage.IndexedMessage) *storage.Database {
	t.Helper()
	db, err := storage.Open(path)
	if err != nil {
		t.Fatalf("open test database %s: %v", path, err)
	}
	if err := db.SyncFiles([]storage.IndexedFile{{
		SourcePath: "/sessions/shared.jsonl",
		Source:     "session",
		Hash:       "hash-" + filepath.Base(path),
		Project:    "project",
		Messages:   messages,
	}}); err != nil {
		_ = db.Close()
		t.Fatalf("sync test database %s: %v", path, err)
	}
	return db
}
