//go:build linux

package recovery

import "golang.org/x/sys/unix"

func noClobberMove(oldPath, newPath string) error {
	err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
	if err == unix.ENOSYS || err == unix.EINVAL {
		return noClobberUnsupportedError{oldPath: oldPath, newPath: newPath, cause: err}
	}
	return err
}
