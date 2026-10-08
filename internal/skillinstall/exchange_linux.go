//go:build linux

package skillinstall

import "golang.org/x/sys/unix"

func exchangePinned(left *pinnedDir, leftName string, right *pinnedDir, rightName string) error {
	return unix.Renameat2(left.fd, leftName, right.fd, rightName, unix.RENAME_EXCHANGE)
}

func moveNoReplacePinned(oldDirectory *pinnedDir, oldName string, newDirectory *pinnedDir, newName string) error {
	return unix.Renameat2(oldDirectory.fd, oldName, newDirectory.fd, newName, unix.RENAME_NOREPLACE)
}
