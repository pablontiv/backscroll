//go:build !darwin && !linux

package skillinstall

import (
	"errors"
	"os"
)

func currentUserID() int { return -1 }

type fileLock struct{}

func acquireLock(string) (*fileLock, error) {
	return nil, errors.New("skill installation is supported only on macOS and Linux")
}

func (*fileLock) close() error { return nil }

func openExclusiveNoFollow(string, os.FileMode) (*os.File, error) {
	return nil, errors.New("skill installation is supported only on macOS and Linux")
}

func openAppendNoFollow(string, os.FileMode) (*os.File, error) {
	return nil, errors.New("skill installation is supported only on macOS and Linux")
}

func exchangePinned(*pinnedDir, string, *pinnedDir, string) error {
	return errors.New("skill installation is supported only on macOS and Linux")
}

func moveNoReplacePinned(*pinnedDir, string, *pinnedDir, string) error {
	return errors.New("skill installation is supported only on macOS and Linux")
}

func linkCount(os.FileInfo) (uint64, error) { return 0, errors.New("POSIX metadata is unavailable") }
func ownerID(os.FileInfo) (int, error)      { return 0, errors.New("POSIX metadata is unavailable") }
func deviceID(os.FileInfo) (uint64, error)  { return 0, errors.New("POSIX metadata is unavailable") }
