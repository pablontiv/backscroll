//go:build windows

package startuplock

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsSidecarKeepsTypeValidationWithoutPOSIXModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(path, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSidecarInfo(path, info); err != nil {
		t.Fatalf("regular Windows file rejected: %v", err)
	}
	dir, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSidecarInfo(path, dir); !errors.Is(err, ErrUnsafeSidecar) {
		t.Fatalf("directory accepted: %v", err)
	}
	link := path + ".link"
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("Windows symlink privilege unavailable: %v", err)
	}
	info, err = os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSidecarInfo(link, info); !errors.Is(err, ErrUnsafeSidecar) {
		t.Fatalf("symlink accepted: %v", err)
	}
}
