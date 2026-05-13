package storage

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// Database represents a SQLite database connection with FTS5 support.
type Database struct {
	db *sql.DB
}

// Open opens or creates a new SQLite database at the given path with FTS5 and WAL mode enabled.
func Open(path string) (*Database, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database %s: %w", path, err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database %s: %w", path, err)
	}

	d := &Database{db: db}
	if err := d.SetupSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

// OpenReadOnly opens an existing SQLite database in read-only mode.
// Fails fast if the database file does not exist.
func OpenReadOnly(path string) (*Database, error) {
	// Fail fast if DB file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("backscroll database not found: %s", path)
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("opening readonly database %s: %w", path, err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping readonly database %s: %w", path, err)
	}

	return &Database{db: db}, nil
}

// Close closes the database connection.
func (d *Database) Close() error {
	return d.db.Close()
}

// DB returns the underlying *sql.DB for direct access (used for embedded migrations).
func (d *Database) DB() *sql.DB {
	return d.db
}
