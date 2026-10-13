//go:build windows

package autoupdate

import (
	"os"
	"os/exec"
)

// Windows cannot overwrite a running executable: move it aside, install the
// prepared copy, and restore it if installation fails.
func atomicReplace(dest, src string) error {
	return replaceWindows(dest, src, windowsReplaceOps{os.Rename, os.Remove})
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

func cleanupBackup(dest string) { _ = os.Remove(dest + ".old") }
