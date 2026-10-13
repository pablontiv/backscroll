//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package startuplock

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type sidecarOwnerInfo struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (i sidecarOwnerInfo) Sys() any { return &i.stat }

func TestSidecarRejectsUnprovenOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid++ // Change only the owner evidence; no privileged chown is needed.
	if err := validateSidecarInfo(path, sidecarOwnerInfo{info, stat}); !errors.Is(err, ErrUnsafeSidecar) {
		t.Fatalf("foreign ownership accepted: %v", err)
	}
}

func TestSidecarFIFOReplacementDoesNotBlock(t *testing.T) {
	db := filepath.Join(t.TempDir(), "index.db")
	path := db + ".startup-sync.lock"
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := newFileLock(db)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if locked, err := lock.TryLock(); err == nil && locked {
		if err := lock.validateLocked(); !errors.Is(err, ErrUnsafeSidecar) {
			t.Fatalf("FIFO accepted: %v", err)
		}
	}
	if lease, locked, err := TryAcquire(db); !errors.Is(err, ErrUnsafeSidecar) || lease != nil || locked {
		t.Fatalf("pre-existing FIFO: lease=%v locked=%v err=%v", lease, locked, err)
	}
}

func TestExistingSidecarPermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o644, 0o666} {
		t.Run(mode.String(), func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "index.db")
			path := db + ".startup-sync.lock"
			if err := os.WriteFile(path, nil, mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			lease, locked, err := TryAcquire(db)
			if mode != 0o600 {
				if lease != nil || locked || !errors.Is(err, ErrUnsafeSidecar) || !strings.Contains(err.Error(), "permissions") {
					if lease != nil {
						_ = lease.Release()
					}
					t.Fatalf("lease=%v locked=%v err=%v", lease, locked, err)
				}
				_, err = Acquire(t.Context(), db, time.Millisecond)
				if !errors.Is(err, ErrUnsafeSidecar) {
					t.Fatalf("Acquire: %v", err)
				}
			} else {
				if err != nil || !locked {
					t.Fatalf("locked=%v err=%v", locked, err)
				}
				defer lease.Release()
				other, busy, err := TryAcquire(db)
				if err != nil || busy || other != nil {
					t.Fatalf("exclusivity: lease=%v locked=%v err=%v", other, busy, err)
				}
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != mode {
				t.Fatalf("mode changed: %v", info.Mode())
			}
		})
	}
}

func TestHeldDescriptorRejectsPermissionChange(t *testing.T) {
	db := filepath.Join(t.TempDir(), "index.db")
	path := db + ".startup-sync.lock"
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := newFileLock(db)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if locked, err := lock.TryLock(); err != nil || !locked {
		t.Fatalf("locked=%v err=%v", locked, err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := lock.validateLocked(); !errors.Is(err, ErrUnsafeSidecar) || !strings.Contains(err.Error(), "permissions") {
		t.Fatalf("changed descriptor mode accepted: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o666 {
		t.Fatalf("unsafe mode silently changed: %v %v", info, err)
	}
}
