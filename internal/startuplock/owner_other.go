//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris

package startuplock

import "os"

// POSIX modes and UID evidence are not available here. Preserve regular-file,
// symlink and identity validation; Windows ACL validation is outside C13.
func validatePrivateSidecar(path string, info os.FileInfo) error { return nil }

func sidecarOpenFlags() int { return os.O_CREATE | os.O_RDONLY }
