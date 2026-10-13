//go:build windows

package autoupdate

import (
	"errors"
	"os"
	"os/exec"
)

// Windows cannot overwrite a running executable: move it aside, install the
// prepared copy, and restore it if installation fails.
func atomicReplace(dest, src string) error {
	backup := dest + ".old"
	if err := os.Rename(dest, backup); err != nil {
		return err
	}
	if err := os.Rename(src, dest); err != nil {
		return errors.Join(err, os.Rename(backup, dest))
	}
	return os.Remove(backup)
}
func platformExec(path string) error {
	cmd := exec.Command(path, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
