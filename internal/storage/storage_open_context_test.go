package storage

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/compat"
)

func TestOpenContextPreCanceledLeavesEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	db, err := OpenContext(ctx, path)
	if db != nil {
		_ = db.Close()
		t.Fatal("OpenContext returned a database for a canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenContext error = %v, want context.Canceled identity", err)
	}
	assertDirectoryEmpty(t, dir)
}

func TestOpenContextCancellationDuringSchemaDoesNotPublish(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")
	ctx, cancel := context.WithCancel(context.Background())
	deps := defaultDatabaseCreationDeps()
	original := deps.migrations[7]
	deps.migrations[7] = func(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
		if err := original(ctx, tx, from); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	}

	db, err := openContextWithCreationDeps(ctx, path, deps)
	if db != nil {
		_ = db.Close()
		t.Fatal("OpenContext returned a database after schema cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenContext error = %v, want context.Canceled identity", err)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("canonical database was published: %v", statErr)
	}
	assertDirectoryEmpty(t, dir)
}

func TestOpenContextCancellationAfterLinkCleansCanonicalFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.db")
	ctx, cancel := context.WithCancel(context.Background())
	deps := defaultDatabaseCreationDeps()
	link := deps.link
	deps.link = func(oldPath, newPath string) error {
		if err := link(oldPath, newPath); err != nil {
			return err
		}
		cancel()
		return nil
	}

	db, err := openContextWithCreationDeps(ctx, path, deps)
	if db != nil {
		_ = db.Close()
		t.Fatal("OpenContext returned a database after publication cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenContext error = %v, want context.Canceled identity", err)
	}
	assertDirectoryEmpty(t, dir)
}

func TestNewDatabaseSchemaFailureRollsBackSingleTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	db, err := openPrivateCreationWithoutSetupContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	injected := errors.New("injected V8 failure")
	migrations := newDatabaseMigrations()
	migrations[7] = func(context.Context, *sql.Tx, compat.SchemaShape) error { return injected }
	if err := db.setupNewDatabaseSchema(context.Background(), migrations); !errors.Is(err, injected) {
		t.Fatalf("setupNewDatabaseSchema error = %v, want injected failure", err)
	}

	var objects int
	if err := db.DB().QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%'
	`).Scan(&objects); err != nil {
		t.Fatal(err)
	}
	if objects != 0 {
		t.Fatalf("schema objects after rollback = %d, want 0", objects)
	}
}

func TestOpenWrapperMatchesOpenContext(t *testing.T) {
	paths := []string{
		filepath.Join(t.TempDir(), "open.db"),
		filepath.Join(t.TempDir(), "open-context.db"),
	}
	dbFromOpen, err := Open(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dbFromOpen.Close() }()
	dbFromContext, err := OpenContext(context.Background(), paths[1])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dbFromContext.Close() }()

	for label, db := range map[string]*Database{"Open": dbFromOpen, "OpenContext": dbFromContext} {
		assertCurrentShape(t, db.DB())
		var versions int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&versions); err != nil {
			t.Fatalf("%s migration count: %v", label, err)
		}
		if versions != 16 {
			t.Fatalf("%s migration count = %d, want 16", label, versions)
		}
	}
}

func TestRecoveryDestinationCancellationCleansAllFiles(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	deps := defaultDatabaseCreationDeps()
	original := deps.migrations[5]
	deps.migrations[5] = func(ctx context.Context, tx *sql.Tx, from compat.SchemaShape) error {
		if err := original(ctx, tx, from); err != nil {
			return err
		}
		cancel()
		return ctx.Err()
	}

	path, err := createRecoveryDestinationWithDeps(ctx, dir, compat.RecoveryPlan{Records: []compat.CanonicalRecord{}}, deps)
	if path != "" {
		t.Fatalf("recovery destination path = %q, want empty after successful cleanup", path)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateRecoveryDestination error = %v, want context.Canceled identity", err)
	}
	assertDirectoryEmpty(t, dir)
}

func assertDirectoryEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("directory entries = %v, want none", directoryEntryNames(entries))
	}
}
