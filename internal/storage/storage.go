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

var openCompatibleApplyMigrationPlan = func(db *Database, ctx context.Context, plan compat.MigrationPlan) error {
	return db.ApplyMigrationPlan(ctx, plan)
}

var ErrImmutableReadOnlyWALUnsafe = errors.New("non-empty WAL makes immutable read-only content unsafe")

// Open opens or creates a new SQLite database at the given path with FTS5 and WAL mode enabled.
func Open(path string) (*Database, error) {
	d, err := openWithoutSetup(path)
	if err != nil {
		return nil, err
	}
	if err := d.SetupSchema(); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

func openWithoutSetup(path string) (*Database, error) {
	return openWriteConnection(path, false)
}

func openMigrationWithoutSetup(path string) (*Database, error) {
	return openWriteConnection(path, true)
}

func openWriteConnection(path string, migrationImmediate bool) (*Database, error) {
	canonicalPath, err := canonicalizeDBPath(path)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite honors the `_pragma=name(value)` DSN syntax; the mattn-style
	// `_name=value` form is silently ignored (leaving rollback journal mode + no busy timeout).
	dsn := canonicalPath + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	if migrationImmediate {
		dsn += "&_txlock=immediate"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", canonicalPath, err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database %s: %w", canonicalPath, err)
	}

	// Enable FK enforcement (required for ON DELETE CASCADE in V2 schema)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	return &Database{db: db, path: canonicalPath}, nil
}

func OpenCompatible(ctx context.Context, path string) (*Database, *compat.Diagnostic, error) {
	inspect, err := OpenReadOnly(path)
	if errors.Is(err, fs.ErrNotExist) {
		db, openErr := Open(path)
		return db, nil, openErr
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
		db, openErr := openWithoutSetup(canonicalPath)
		return db, nil, openErr
	}

	migrationDB, err := openMigrationWithoutSetup(canonicalPath)
	if err != nil {
		return nil, nil, err
	}
	if err := openCompatibleApplyMigrationPlan(migrationDB, ctx, plan); err != nil {
		_ = migrationDB.Close()
		return nil, nil, err
	}
	if err := migrationDB.Close(); err != nil {
		return nil, nil, err
	}
	db, err := openWithoutSetup(canonicalPath)
	if err != nil {
		return nil, nil, err
	}
	return db, nil, nil
}

// OpenReadOnly opens an existing SQLite database in read-only mode.
// Fails fast if the database file does not exist.
func OpenReadOnly(path string) (*Database, error) {
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

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping readonly database %s: %w", canonicalPath, err)
	}

	return &Database{db: db, path: canonicalPath}, nil
}

// OpenImmutableReadOnly opens an existing SQLite database without creating or
// touching SQLite sidecar files. It refuses non-empty WAL files because an
// immutable view can miss committed frames that are not checkpointed into the
// main database file.
func OpenImmutableReadOnly(path string) (*Database, error) {
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
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping immutable readonly database %s: %w", canonicalPath, err)
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
