//go:build darwin

package recovery

import "golang.org/x/sys/unix"

func noClobberMove(oldPath, newPath string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_EXCL)
}
