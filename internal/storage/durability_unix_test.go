//go:build darwin || linux

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDurabilityUnixErrorsSurface(t *testing.T) {
	// A device rejects fsync with EINVAL on Darwin/Linux; do not suppress it.
	if err := fsyncDirectory("/dev/null"); err == nil {
		t.Fatal("device fsync error was suppressed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable")
	if err := os.WriteFile(path, []byte("keep"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	f, err := os.Open(path)
	if err == nil {
		_ = f.Close()
		t.Skip("host privileges bypass file permission controls")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal(err)
	}
	if err := fsyncPath(path); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("file permission error = %v", err)
	}
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if err := fsyncDirectory(dir); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("directory permission error = %v", err)
	}
}
