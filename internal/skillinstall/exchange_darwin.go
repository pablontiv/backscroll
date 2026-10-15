//go:build darwin

package skillinstall

import "golang.org/x/sys/unix"

func exchangePinned(left *pinnedDir, leftName string, right *pinnedDir, rightName string) error {
	return unix.RenameatxNp(left.fd, leftName, right.fd, rightName, unix.RENAME_SWAP)
}

func moveNoReplacePinned(oldDirectory *pinnedDir, oldName string, newDirectory *pinnedDir, newName string) error {
	return unix.RenameatxNp(oldDirectory.fd, oldName, newDirectory.fd, newName, unix.RENAME_EXCL)
}
