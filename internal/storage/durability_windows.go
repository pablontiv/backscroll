//go:build windows

package storage

import "os"

func openSyncFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR, 0)
}

// SyncDirectory is unsupported on Windows and intentionally does nothing.
// File contents are flushed separately; rename/directory crash durability is
// not guaranteed by this primitive on Windows.
func SyncDirectory(path string) error { return nil }
