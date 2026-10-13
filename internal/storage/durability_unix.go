//go:build !windows

package storage

import (
	"errors"
	"os"
)

func openSyncFile(path string) (*os.File, error) { return os.Open(path) }

// SyncDirectory flushes directory metadata and surfaces all open/sync/close
// errors, including unsupported filesystem errors on Darwin and Linux.
func SyncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
