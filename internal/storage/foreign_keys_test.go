package storage

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"modernc.org/sqlite"
)

// A one-time PRAGMA on sql.DB leaves later physical connections unprotected.
func TestWritableConnectionsEnforceForeignKeys(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		open func(string) (*Database, error)
	}{
		{"Open", Open},
		{"OpenCompatible", func(path string) (*Database, error) {
			db, diagnostic, err := OpenCompatible(ctx, path)
			if diagnostic != nil {
				t.Fatalf("compatibility diagnostic: %v", diagnostic)
			}
			return db, err
		}},
		{"migration", func(path string) (*Database, error) {
			return openMigrationWithoutSetupContext(ctx, path)
		}},
		{"private_creation", func(path string) (*Database, error) {
			return openPrivateCreationWithoutSetupContext(ctx, path)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "foreign_keys.db")
			seed, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := seed.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := tc.open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			pool := db.DB()
			pool.SetMaxOpenConns(2)
			pool.SetMaxIdleConns(2)
			check := func(conn *sql.Conn) {
				t.Helper()
				var enabled int
				if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
					t.Fatal(err)
				}
				if enabled != 1 {
					t.Errorf("foreign_keys = %d, want 1", enabled)
				}
				_, err := conn.ExecContext(ctx, "INSERT INTO embedding_metadata (chunk_id, model_name, model_version, dimensions, created_at) VALUES (-1, 'test', '1', 3, 0)")
				var sqliteErr *sqlite.Error
				if !errors.As(err, &sqliteErr) || sqliteErr.Code() != 787 {
					t.Errorf("orphan insert error = %v, want SQLITE_CONSTRAINT_FOREIGNKEY (787)", err)
				}
			}
			// Hold both leases simultaneously so the second is a new physical connection.
			first, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			second, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			check(first)
			check(second)
			if err := second.Close(); err != nil {
				t.Fatal(err)
			}
			reused, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			check(reused)
			if err := reused.Close(); err != nil {
				t.Fatal(err)
			}
			// Discard idle connections; the next lease must initialize a replacement.
			pool.SetMaxIdleConns(0)
			replacement, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer replacement.Close()
			check(replacement)
		})
	}
}
