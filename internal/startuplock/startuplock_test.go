package startuplock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTryAcquireCanonicalAliasesContend(t *testing.T) {
	dir := t.TempDir()
	realDB := filepath.Join(dir, "index.db")
	if err := os.WriteFile(realDB, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias.db")
	if err := os.Symlink(realDB, alias); err != nil {
		t.Fatal(err)
	}

	first, locked, err := TryAcquire(realDB)
	if err != nil || !locked {
		t.Fatalf("first acquire locked=%v err=%v", locked, err)
	}
	defer first.Release()

	second, locked, err := TryAcquire(alias)
	if err != nil {
		t.Fatalf("alias acquire: %v", err)
	}
	if locked || second != nil {
		t.Fatalf("alias bypassed canonical lock: lease=%v locked=%v", second, locked)
	}
}

func TestSidecarCreatedPrivateAndPersists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	lease, locked, err := TryAcquire(dbPath)
	if err != nil || !locked {
		t.Fatalf("acquire locked=%v err=%v", locked, err)
	}
	path, err := sidecarPath(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("permissions=%#o want 0600", got)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sidecar removed after release: %v", err)
	}
}

func TestTryAcquireRejectsUnsafeSidecars(t *testing.T) {
	for _, kind := range []string{"directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			dbPath := filepath.Join(dir, "index.db")
			path := dbPath + ".startup-sync.lock"
			if kind == "directory" {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				target := filepath.Join(dir, "target")
				if err := os.WriteFile(target, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			_, _, err := TryAcquire(dbPath)
			if !errors.Is(err, ErrUnsafeSidecar) {
				t.Fatalf("error=%v want ErrUnsafeSidecar", err)
			}
		})
	}
}

func TestAcquireHonorsCanceledContext(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	owner, locked, err := TryAcquire(dbPath)
	if err != nil || !locked {
		t.Fatalf("owner locked=%v err=%v", locked, err)
	}
	defer owner.Release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = Acquire(ctx, dbPath, time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestLeaseReleaseIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	lease, locked, err := TryAcquire(dbPath)
	if err != nil || !locked {
		t.Fatalf("locked=%v err=%v", locked, err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	next, locked, err := TryAcquire(dbPath)
	if err != nil || !locked {
		t.Fatalf("reacquire locked=%v err=%v", locked, err)
	}
	defer next.Release()
}

func TestTryAcquireMissingParentReturnsErrorAndDoesNotCreateDirectory(t *testing.T) {
	dir := t.TempDir()
	missingParent := filepath.Join(dir, "missing")
	dbPath := filepath.Join(missingParent, "index.db")

	_, _, err := TryAcquire(dbPath)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "resolve database parent") {
		t.Fatalf("error=%v want resolve database parent", err)
	}
	if _, statErr := os.Stat(missingParent); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("missing parent stat error=%v want not exist", statErr)
	}
}

// Stage replacements at exact acquisition boundaries without timing-dependent
// goroutines or production hooks. validateLocked uses flock's held descriptor.
func TestSidecarReplacementDuringAcquisition(t *testing.T) {
	for _, stage := range []string{"before_open", "after_open"} {
		for _, kind := range []string{"private", "public", "symlink", "directory"} {
			t.Run(stage+"/"+kind, func(t *testing.T) {
				if runtime.GOOS == "windows" && kind == "public" {
					t.Skip("POSIX modes do not describe Windows ACLs")
				}
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
				if stage == "after_open" {
					if locked, err := lock.TryLock(); err != nil || !locked {
						t.Fatalf("locked=%v err=%v", locked, err)
					}
				}
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "private", "public":
					mode := os.FileMode(0o600)
					if kind == "public" {
						mode = 0o666
					}
					if err := os.WriteFile(path, nil, mode); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(path, mode); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					if err := os.Symlink(path+".original", path); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Mkdir(path, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				opened := stage == "after_open"
				if stage == "before_open" {
					locked, err := lock.TryLock()
					if err != nil && kind != "symlink" && kind != "directory" {
						t.Fatalf("unexpected open failure: %v", err)
					}
					if err == nil && !locked {
						t.Fatal("replacement unexpectedly busy")
					}
					opened = err == nil && locked
				}
				if opened {
					if err := lock.validateLocked(); !errors.Is(err, ErrUnsafeSidecar) {
						t.Fatalf("validation=%v", err)
					}
				}
				if err := lock.Close(); err != nil {
					t.Fatal(err)
				}
				// The rejected descriptor must not leave the original inode locked.
				originalDB := db + ".original-db"
				if err := os.Rename(path+".original", originalDB+".startup-sync.lock"); err != nil {
					t.Fatal(err)
				}
				lease, locked, err := TryAcquire(originalDB)
				if err != nil || !locked {
					t.Fatalf("original still locked: %v %v", locked, err)
				}
				if err := lease.Release(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
