//go:build darwin

package skillinstall

import "golang.org/x/sys/unix"

func exchangePaths(left, right string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, left, unix.AT_FDCWD, right, unix.RENAME_SWAP)
}

func moveNoReplace(oldPath, newPath string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_EXCL)
}
