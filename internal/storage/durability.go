package storage

import "errors"

// SyncFile flushes an existing regular file without creating, truncating, or
// changing its contents. Windows needs a writable handle for FlushFileBuffers;
// other platforms retain read-only open semantics and their permission errors.
func SyncFile(path string) error {
	file, err := openSyncFile(path)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
