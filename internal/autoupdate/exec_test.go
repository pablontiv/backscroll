package autoupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlatformExecMissingBinary(t *testing.T) {
	// Missing path guarantees no real process replacement/spawn occurs.
	if err := platformExec(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("execution error swallowed")
	}
}
func TestAtomicReplaceFilesystemError(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "installed")
	if err := os.WriteFile(dest, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := atomicReplace(dest, filepath.Join(dir, "missing")); err == nil {
		t.Fatal("replace error swallowed")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "old" {
		t.Fatalf("old installation lost: %q %v", data, err)
	}
}
