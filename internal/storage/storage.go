package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/pablontiv/backscroll/internal/compat"
	"github.com/pablontiv/backscroll/internal/embedding"
)

// Database represents a SQLite database connection with FTS5 support.
type Database struct {
	db                *sql.DB
	path              string
	embeddingProvider embedding.EmbeddingProvider
}

var (
	openCompatibleSnapshotDatabase = func(ctx context.Context, path string, plan compat.MigrationPlan) (string, error) {
		return snapshotDatabase(ctx, path, plan)
	}
	openCompatibleApplyMigrationPlan = applyMigrationPlanLocked
)

type databaseCreationDeps struct {
	openPrivate   func(context.Context, string) (*Database, error)
	openCanonical func(context.Context, string) (*Database, error)
	migrations    []migrationApplier
	link          func(string, string) error
}

func defaultDatabaseCreationDeps() databaseCreationDeps {
	return databaseCreationDeps{
		openPrivate:   openPrivateCreationWithoutSetupContext,
		openCanonical: openWithoutSetupContext,
		migrations:    newDatabaseMigrations(),
		link:          os.Link,
	}
}

var ErrImmutableReadOnlyWALUnsafe = errors.New("non-empty WAL makes immutable read-only content unsafe")

// Open opens or creates a SQLite database at the given path with FTS5 and WAL mode enabled.
// Existing databases always pass through compatibility inspection and backed-up migration.
func Open(path string) (*Database, error) {
	return OpenContext(context.Background(), path)
}

// OpenContext is Open with cancellation propagated through connection setup,
// private schema construction, publication, and existing-database migration.
func OpenContext(ctx context.Context, path string) (*Database, error) {
	if ctx == nil {
		return nil, errors.New("open database: nil context")
	}
	return openContextWithCreationDeps(ctx, path, defaultDatabaseCreationDeps())
}

func openContextWithCreationDeps(ctx context.Context, path string, deps databaseCreationDeps) (*Database, error) {
	db, created, err := createDatabaseExclusively(ctx, path, deps)
	if err != nil || created {
		return db, contextIdentityError(ctx, err)
	}

	db, diag, openErr := OpenCompatible(ctx, path)
	if diag != nil {
		if db != nil {
			_ = db.Close()
		}
		return nil, fmt.Errorf("%s: %s", diag.Code, diag.Summary)
	}
	return db, contextIdentityError(ctx, openErr)
}

func createDatabaseExclusively(ctx context.Context, path string, deps databaseCreationDeps) (db *Database, created bool, err error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	canonicalPath, err := canonicalizeDBPath(path)
	if err != nil {
		return nil, false, err
	}
	if _, err := os.Stat(canonicalPath); err == nil {
		return nil, false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, fmt.Errorf("stat database path %s: %w", canonicalPath, err)
	}

	directory := filepath.Dir(canonicalPath)
	published := false
	defer func() {
		if err == nil || !published {
			return
		}
		if db != nil {
			err = errors.Join(err, db.Close())
			db = nil
		}
		if cleanupErr := cleanupCreationFiles(canonicalPath); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup failed database publication: %w", cleanupErr))
		}
		if syncErr := fsyncDirectory(directory); syncErr != nil {
			err = errors.Join(err, syncErr)
		}
	}()

	temp, err := os.CreateTemp(directory, "."+filepath.Base(canonicalPath)+".create-*")
	if err != nil {
		return nil, false, fmt.Errorf("create private database file for %s: %w", canonicalPath, err)
	}
	tempPath := temp.Name()
	defer func() {
		if tempPath == "" {
			return
		}
		if cleanupErr := cleanupCreationFiles(tempPath); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if err := temp.Close(); err != nil {
		return nil, false, fmt.Errorf("close private database file %s: %w", tempPath, err)
	}

	candidate, err := createDatabaseWithOpen(ctx, tempPath, deps.openPrivate, deps.migrations)
	if err != nil {
		return nil, false, fmt.Errorf("initialize private database for %s: %w", canonicalPath, err)
	}
	if err := candidate.Close(); err != nil {
		return nil, false, fmt.Errorf("close initialized private database %s: %w", tempPath, err)
	}
	if err := ensureCreationHasNoSidecars(tempPath); err != nil {
		return nil, false, err
	}
	if err := fsyncPath(tempPath); err != nil {
		return nil, false, err
	}

	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if err := deps.link(tempPath, canonicalPath); err != nil {
		if errors.Is(err, fs.ErrExist) {
			if cleanupErr := cleanupCreationFiles(tempPath); cleanupErr != nil {
				return nil, false, cleanupErr
			}
			tempPath = ""
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("publish database without clobbering %s: %w", canonicalPath, err)
	}
	published = true
	if err := fsyncDirectory(directory); err != nil {
		return nil, true, err
	}
	if err := cleanupCreationFiles(tempPath); err != nil {
		return nil, true, err
	}
	tempPath = ""
	if err := fsyncDirectory(directory); err != nil {
		return nil, true, err
	}

	// Reopen through the canonical name so the returned connection never depends
	// on the private construction link and uses the ordinary WAL configuration.
	db, err = deps.openCanonical(ctx, canonicalPath)
	return db, true, contextIdentityError(ctx, err)
}

func createDatabaseWithOpen(ctx context.Context, path string, open func(context.Context, string) (*Database, error), migrations []migrationApplier) (*Database, error) {
	d, err := open(ctx, path)
	if err != nil {
		return nil, contextIdentityError(ctx, err)
	}
	if err := d.setupNewDatabaseSchema(ctx, migrations); err != nil {
		closeErr := d.Close()
		return nil, errors.Join(contextIdentityError(ctx, err), closeErr)
	}
	return d, nil
}

func contextIdentityError(ctx context.Context, err error) error {
	if err == nil || ctx == nil || ctx.Err() == nil || errors.Is(err, ctx.Err()) {
		return err
	}
	return errors.Join(err, ctx.Err())
}

func cleanupCreationFiles(path string) error {
	var cleanupErr error
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove private database file %s: %w", path+suffix, err))
		}
	}
	return cleanupErr
}

func ensureCreationHasNoSidecars(path string) error {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if _, err := os.Lstat(path + suffix); err == nil {
			return fmt.Errorf("private database construction left sidecar %s", path+suffix)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("inspect private database sidecar %s: %w", path+suffix, err)
		}
	}
	return nil
}

func openWithoutSetup(path string) (*Database, error) {
	return openWithoutSetupContext(context.Background(), path)
}

func openWithoutSetupContext(ctx context.Context, path string) (*Database, error) {
	return openWriteConnectionContext(ctx, path, false)
}

func openPrivateCreationWithoutSetupContext(ctx context.Context, path string) (*Database, error) {
	return openWriteConnectionWithPragmasContext(ctx, path, false, "DELETE", "FULL")
}

func openMigrationWithoutSetupContext(ctx context.Context, path string) (*Database, error) {
	return openWriteConnectionContext(ctx, path, true)
}

func openWriteConnectionContext(ctx context.Context, path string, migrationImmediate bool) (*Database, error) {
	return openWriteConnectionWithPragmasContext(ctx, path, migrationImmediate, "WAL", "NORMAL")
}

func openWriteConnectionWithPragmasContext(ctx context.Context, path string, migrationImmediate bool, journalMode, synchronous string) (*Database, error) {
	canonicalPath, err := canonicalizeDBPath(path)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite honors the `_pragma=name(value)` DSN syntax; the mattn-style
	// `_name=value` form is silently ignored (leaving rollback journal mode + no busy timeout).
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(%s)&_pragma=synchronous(%s)&_pragma=busy_timeout(5000)", canonicalPath, journalMode, synchronous)
	if migrationImmediate {
		dsn += "&_txlock=immediate"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", canonicalPath, err)
	}

	// Test the connection using the caller's cancellation scope.
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, contextIdentityError(ctx, fmt.Errorf("ping database %s: %w", canonicalPath, err))
	}

	// Enable FK enforcement (required for ON DELETE CASCADE in V2 schema).
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, contextIdentityError(ctx, fmt.Errorf("enable foreign keys: %w", err))
	}

	return &Database{db: db, path: canonicalPath}, nil
}

func OpenCompatible(ctx context.Context, path string) (*Database, *compat.Diagnostic, error) {
	if ctx == nil {
		return nil, nil, errors.New("open compatible database: nil context")
	}
	inspect, err := openReadOnlyContext(ctx, path)
	if errors.Is(err, fs.ErrNotExist) {
		db, created, createErr := createDatabaseExclusively(ctx, path, defaultDatabaseCreationDeps())
		if createErr != nil || created {
			return db, nil, createErr
		}
		// Another opener won the exclusive creation race. Treat its path as an
		// existing database and inspect it rather than setting up over it.
		inspect, err = openReadOnlyContext(ctx, path)
	}
	if err != nil {
		return nil, nil, err
	}
	canonicalPath := inspect.path
	plan, diag, err := compat.InspectIndex(ctx, inspect.DB())
	closeErr := inspect.Close()
	if err != nil || diag != nil {
		return nil, diag, err
	}
	if closeErr != nil {
		return nil, nil, closeErr
	}
	if len(plan.Steps) == 0 {
		db, openErr := openWithoutSetupContext(ctx, canonicalPath)
		return db, nil, openErr
	}

	migrationDB, err := openMigrationWithoutSetupContext(ctx, canonicalPath)
	if err != nil {
		return nil, nil, err
	}
	migrationClosed := false
	defer func() {
		if !migrationClosed {
			_ = migrationDB.Close()
		}
	}()

	tx, err := beginMigrationTx(ctx, migrationDB.db)
	if err != nil {
		return nil, nil, fmt.Errorf("begin migration plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := validateMigrationPlanLocked(ctx, tx, plan); err != nil {
		return nil, nil, err
	}
	if _, err := openCompatibleSnapshotDatabase(ctx, canonicalPath, plan); err != nil {
		return nil, nil, fmt.Errorf("snapshot database before migration: %w", err)
	}
	if err := openCompatibleApplyMigrationPlan(ctx, tx, plan); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit migration plan: %w", err)
	}
	if err := migrationDB.Close(); err != nil {
		return nil, nil, err
	}
	migrationClosed = true

	db, err := openWithoutSetupContext(ctx, canonicalPath)
	if err != nil {
		return nil, nil, err
	}
	if err := compat.VerifyCurrentShape(ctx, db.DB()); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("verify committed schema: %w", err)
	}
	return db, nil, nil
}

// OpenReadOnly opens an existing SQLite database in read-only mode.
// Fails fast if the database file does not exist.
func OpenReadOnly(path string) (*Database, error) {
	return openReadOnlyContext(context.Background(), path)
}

func openReadOnlyContext(ctx context.Context, path string) (*Database, error) {
	// Fail fast if DB file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("backscroll database not found: %s: %w", path, fs.ErrNotExist)
	}
	canonicalPath, err := canonicalizeExistingDBPath(path)
	if err != nil {
		return nil, err
	}

	// Journal mode is persisted in the DB file (set by the write connection); a read-only
	// connection only needs the busy timeout so queries wait out a concurrent writer's lock.
	db, err := sql.Open("sqlite", "file:"+canonicalPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening readonly database %s: %w", canonicalPath, err)
	}

	// Test the connection using the caller's cancellation scope.
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, contextIdentityError(ctx, fmt.Errorf("ping readonly database %s: %w", canonicalPath, err))
	}

	return &Database{db: db, path: canonicalPath}, nil
}

// OpenImmutableReadOnly opens an existing SQLite database without creating or
// touching SQLite sidecar files. It refuses non-empty WAL files because an
// immutable view can miss committed frames that are not checkpointed into the
// main database file.
func OpenImmutableReadOnly(path string) (*Database, error) {
	return openImmutableReadOnlyContext(context.Background(), path)
}

func openImmutableReadOnlyContext(ctx context.Context, path string) (*Database, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("backscroll database not found: %s: %w", path, fs.ErrNotExist)
	}
	canonicalPath, err := canonicalizeExistingDBPath(path)
	if err != nil {
		return nil, err
	}
	walPath := canonicalPath + "-wal"
	if wal, err := os.Stat(walPath); err == nil {
		if wal.Size() > 0 {
			return nil, fmt.Errorf("%w: %s", ErrImmutableReadOnlyWALUnsafe, canonicalPath)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat WAL sidecar %s: %w", walPath, err)
	}

	db, err := sql.Open("sqlite", "file:"+canonicalPath+"?mode=ro&immutable=1&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening immutable readonly database %s: %w", canonicalPath, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, contextIdentityError(ctx, fmt.Errorf("ping immutable readonly database %s: %w", canonicalPath, err))
	}
	return &Database{db: db, path: canonicalPath}, nil
}

func canonicalizeDBPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize database path %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err == nil {
		return canonicalizeExistingDBPath(abs)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat database path %s: %w", abs, err)
	}
	return abs, nil
}

func canonicalizeExistingDBPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize database path %s: %w", path, err)
	}
	realPath, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve database path %s: %w", abs, err)
	}
	return realPath, nil
}

// Close closes the database connection.
func (d *Database) Close() error {
	return d.db.Close()
}

// DB returns the underlying *sql.DB for direct access (used for embedded migrations).
func (d *Database) DB() *sql.DB {
	return d.db
}
