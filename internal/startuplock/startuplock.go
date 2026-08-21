package startuplock

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

var ErrUnsafeSidecar = errors.New("unsafe startup lock sidecar")

type Lease struct {
	mu       sync.Mutex
	lock     *flock.Flock
	released bool
}

func canonicalDatabasePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize database path %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err == nil {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("resolve database path %s: %w", abs, err)
		}
		return resolved, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("stat database path %s: %w", abs, err)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("resolve database parent %s: %w", filepath.Dir(abs), err)
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func sidecarPath(databasePath string) (string, error) {
	canonical, err := canonicalDatabasePath(databasePath)
	if err != nil {
		return "", err
	}
	return canonical + ".startup-sync.lock", nil
}

func validateExistingSidecar(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect startup lock %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrUnsafeSidecar, path)
	}
	return nil
}

func newFileLock(databasePath string) (*flock.Flock, error) {
	path, err := sidecarPath(databasePath)
	if err != nil {
		return nil, err
	}
	if err := validateExistingSidecar(path); err != nil {
		return nil, err
	}
	return flock.New(path, flock.SetPermissions(0o600)), nil
}

func TryAcquire(databasePath string) (*Lease, bool, error) {
	fileLock, err := newFileLock(databasePath)
	if err != nil {
		return nil, false, err
	}
	locked, err := fileLock.TryLock()
	if err != nil {
		return nil, false, errors.Join(err, fileLock.Close())
	}
	if !locked {
		return nil, false, fileLock.Close()
	}
	return &Lease{lock: fileLock}, true, nil
}

func Acquire(ctx context.Context, databasePath string, retryDelay time.Duration) (*Lease, error) {
	if retryDelay <= 0 {
		return nil, fmt.Errorf("startup lock retry delay must be positive")
	}
	fileLock, err := newFileLock(databasePath)
	if err != nil {
		return nil, err
	}
	locked, err := fileLock.TryLockContext(ctx, retryDelay)
	if err != nil {
		return nil, errors.Join(err, fileLock.Close())
	}
	if !locked {
		return nil, errors.Join(ctx.Err(), fileLock.Close())
	}
	return &Lease{lock: fileLock}, nil
}

func (l *Lease) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	if err := l.lock.Close(); err != nil {
		return err
	}
	l.released = true
	return nil
}
