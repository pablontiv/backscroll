//go:build !windows

package autoupdate

import (
	"os"
	"syscall"
)

func atomicReplace(dest, src string) error { return os.Rename(src, dest) }
func platformExec(path string) error       { return syscall.Exec(path, os.Args, os.Environ()) }

func cleanupBackup(string) {}
