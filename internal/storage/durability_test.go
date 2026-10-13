package storage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDurabilityFilePreservesBytesAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot")
	want := []byte("snapshot bytes\x00must not be truncated")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		err = f.Sync()
		_ = f.Close()
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("read-only Windows Sync = %v, want permission error", err)
		}
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := fsyncPath(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("bytes changed: %q, %v", got, err)
	}
	after, err := os.Stat(path)
	if err != nil || after.Mode() != before.Mode() {
		t.Fatalf("sync changed permissions: before=%v after=%v err=%v", before, after, err)
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	if runtime.GOOS == "windows" {
		if err := fsyncPath(path); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("read-only file sync = %v, want permission error", err)
		}
	} else if err := fsyncPath(path); err != nil {
		t.Fatalf("Unix read-only file sync: %v", err)
	}
	if err := fsyncPath(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file sync = %v", err)
	}
}

func TestDurabilityDirectoryPolicy(t *testing.T) {
	if err := fsyncDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	err := fsyncDirectory(filepath.Join(t.TempDir(), "missing"))
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Windows unsupported directory sync = %v", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory sync = %v", err)
	}
}

func TestDurabilityV14MigrationPreservesRowsAndSnapshot(t *testing.T) {
	path := createFixtureDatabase(t, "v14.sql")
	want := seedFixtureSentinels(t, path)
	db, diag, err := OpenCompatible(context.Background(), path)
	if err != nil || diag != nil {
		t.Fatalf("migration: %v, %+v", err, diag)
	}
	defer db.Close()
	assertCurrentShape(t, db.DB())
	assertSearchItems(t, db.DB(), want.SearchItems)
	assertSearchItemsByUUID(t, db.DB(), want.ToolSearchItems)
	assertTableSentinels(t, db.DB(), want)
	snapshot, err := OpenReadOnly(onlySnapshot(t, path))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	assertSearchItems(t, snapshot.DB(), want.SearchItems)
	assertSearchItemsByUUID(t, snapshot.DB(), want.ToolSearchItems)
	assertTableSentinels(t, snapshot.DB(), want)
}
