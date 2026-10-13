//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package startuplock

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

func sidecarOwnedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint64(stat.Uid) == uint64(os.Geteuid())
}

// Refuse symlink traversal and avoid blocking if a FIFO replaces the sidecar.
func sidecarOpenFlags() int {
	flags := os.O_CREATE | os.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if runtime.GOOS == "aix" || runtime.GOOS == "solaris" || runtime.GOOS == "illumos" {
		flags = os.O_CREATE | os.O_RDWR | unix.O_NOFOLLOW | unix.O_NONBLOCK
	}
	return flags
}

func validatePrivateSidecar(path string, info os.FileInfo) error {
	if info.Mode().Perm() != 0o600 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("%w: %s permissions %s; require 0600", ErrUnsafeSidecar, path, info.Mode())
	}
	if !sidecarOwnedByCurrentUser(info) {
		return fmt.Errorf("%w: %s ownership is not the current user", ErrUnsafeSidecar, path)
	}
	return nil
}
