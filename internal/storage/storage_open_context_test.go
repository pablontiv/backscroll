package storage

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestOpenContextCancellationAfterLinkReturnsPublishedDatabase(t *testing.T) {
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
	if err != nil {
		t.Fatalf("OpenContext after publication cancellation: %v", err)
	}
	if db == nil {
		t.Fatal("OpenContext returned nil published database")
	}
	defer func() { _ = db.Close() }()
	assertCurrentShape(t, db.DB())
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("published canonical database missing: %v", err)
	}
}

func TestPublishedDatabaseSurvivesWinnerCancellationWhileLoserOpens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "race ? # %.db")
	ctx, cancel := context.WithCancel(context.Background())
	deps := defaultDatabaseCreationDeps()
	link := deps.link
	published := make(chan struct{})
	releaseWinner := make(chan struct{})
	deps.link = func(oldPath, newPath string) error {
		if err := link(oldPath, newPath); err != nil {
			return err
		}
		close(published)
		<-releaseWinner
		return nil
	}

	type result struct {
		db  *Database
		err error
	}
	winnerResult := make(chan result, 1)
	go func() {
		db, err := openContextWithCreationDeps(ctx, path, deps)
		winnerResult <- result{db: db, err: err}
	}()

	<-published
	cancel()
	loser, loserErr := OpenContext(context.Background(), path)
	close(releaseWinner)
	winner := <-winnerResult
	if loserErr != nil {
		t.Fatalf("loser open after publication: %v", loserErr)
	}
	if winner.err != nil {
		if loser != nil {
			_ = loser.Close()
		}
		t.Fatalf("winner after post-publication cancellation: %v", winner.err)
	}
	if loser == nil || winner.db == nil {
		t.Fatalf("published opens returned nil handles: loser=%v winner=%v", loser, winner.db)
	}
	defer func() { _ = loser.Close() }()
	defer func() { _ = winner.db.Close() }()
	assertCurrentShape(t, loser.DB())
	assertCurrentShape(t, winner.db.DB())

	readonly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("open final canonical database: %v", err)
	}
	defer func() { _ = readonly.Close() }()
	assertCurrentShape(t, readonly.DB())
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".create-") {
			t.Fatalf("private candidate survived publication: %s", entry.Name())
		}
	}
}

func TestExistingWriteOpenCannotRecreateDisappearedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing ? # %.db")
	db, err := openWithoutSetupContext(context.Background(), path)
	if db != nil {
		_ = db.Close()
		t.Fatal("existing-database open created a handle for a missing path")
	}
	if err == nil {
		t.Fatal("existing-database open succeeded for a missing path")
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("existing-database open created an empty file: %v", statErr)
	}
}

func TestOpenContextPreCanceledExistingDatabasePreservesSidecars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	sidecar := path + "-journal"
	wantSidecar := []byte("owned by existing database")
	if err := os.WriteFile(sidecar, wantSidecar, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := OpenContext(ctx, path)
	if got != nil {
		_ = got.Close()
		t.Fatal("pre-canceled existing open returned a database")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenContext error = %v, want context.Canceled", err)
	}
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("existing sidecar was removed: %v", err)
	}
	if string(data) != string(wantSidecar) {
		t.Fatalf("existing sidecar changed: got %q want %q", data, wantSidecar)
	}
}

func TestEEXISTLoserCleansOnlyPrivateCandidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adopted.db")
	deps := defaultDatabaseCreationDeps()
	link := deps.link
	wantSidecar := []byte("adopted sidecar")
	deps.link = func(oldPath, newPath string) error {
		if err := link(oldPath, newPath); err != nil {
			return err
		}
		if err := os.WriteFile(newPath+"-journal", wantSidecar, 0o600); err != nil {
			return err
		}
		return fs.ErrExist
	}

	db, created, err := createDatabaseExclusively(context.Background(), path, deps)
	if err != nil || created || db != nil {
		t.Fatalf("EEXIST loser result db=%v created=%v err=%v", db, created, err)
	}
	data, err := os.ReadFile(path + "-journal")
	if err != nil {
		t.Fatalf("adopted sidecar missing: %v", err)
	}
	if string(data) != string(wantSidecar) {
		t.Fatalf("adopted sidecar changed: got %q want %q", data, wantSidecar)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(path) && entry.Name() != filepath.Base(path)+"-journal" {
			t.Fatalf("private candidate leaked: %s", entry.Name())
		}
	}
	if err := os.Remove(path + "-journal"); err != nil {
		t.Fatal(err)
	}
	readonly, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("adopted canonical database is incomplete: %v", err)
	}
	defer func() { _ = readonly.Close() }()
	assertCurrentShape(t, readonly.DB())
}

func TestNewDatabaseSchemaFailureRollsBackSingleTransaction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
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

func TestPublishedDatabaseHasPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private ? # %.db")
	db, err := OpenContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if supportsPOSIXModes() && info.Mode().Perm() != 0o600 {
		t.Fatalf("canonical mode = %o, want 600", info.Mode().Perm())
	}
	assertCurrentShape(t, db.DB())
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
