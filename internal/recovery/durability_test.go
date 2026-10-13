package recovery

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pablontiv/backscroll/internal/storage"
)

func TestDurabilityRecoveryFileSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-copy.db")
	want := []byte("verified recovery copy\x00and backup bytes")
	if err := os.WriteFile(path, want, 0600); err != nil {
		t.Fatal(err)
	}
	if err := syncFile(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("sync changed bytes: %q, %v", got, err)
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	err = syncFile(path)
	if runtime.GOOS == "windows" {
		if !errors.Is(err, os.ErrPermission) {
			t.Fatalf("read-only sync = %v, want permission error", err)
		}
	} else if err != nil {
		t.Fatalf("Unix read-only sync = %v", err)
	}
	if err := syncFile(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file sync = %v", err)
	}
}

func TestDurabilityRecoveryDirectoryPolicy(t *testing.T) {
	if err := syncDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	err := syncDir(filepath.Join(t.TempDir(), "missing"))
	if runtime.GOOS == "windows" {
		if err != nil {
			t.Fatalf("Windows directory no-op = %v", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing directory sync = %v", err)
	}
}

func TestDurabilityRecoverySnapshotCopy(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	createRecoveryDB(t, source, []storage.IndexedMessage{{Ordinal: 0, Role: "user", Text: "preserved recovery snapshot", UUID: "durability-copy", ContentType: "text"}})
	before := snapshotRecoveryDBFile(t, source)
	copyPath := filepath.Join(dir, "verified-copy.db")
	if err := os.WriteFile(copyPath, before.main.data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := syncFile(copyPath); err != nil {
		t.Fatal(err)
	}
	if err := syncDir(dir); err != nil {
		t.Fatal(err)
	}
	copy := snapshotRecoveryDBFile(t, copyPath)
	if !bytes.Equal(copy.main.data, before.main.data) {
		t.Fatal("synchronized snapshot copy differs from source")
	}
	assertRecoveryTexts(t, copyPath, []string{"preserved recovery snapshot"})
	assertRecoveryDBFileSnapshot(t, "source after snapshot copy", source, before)
}
