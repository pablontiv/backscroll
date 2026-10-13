package autoupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pablontiv/picokit/hashfile"
)

// ApplyStagedIfAvailable retains the staged candidate on every failure,
// including replace/reexec. The installed binary is replaced from a separate
// copy, so a failed reexec can be retried on the next invocation.
func (u *Updater) ApplyStagedIfAvailable() error {
	if u.CurrentVersion == "dev" {
		return nil
	}
	cache, err := u.cacheDir()
	if err != nil {
		return fmt.Errorf("cache directory: %w", err)
	}
	base := filepath.Join(cache, u.Binary, "staged")
	entries, err := os.ReadDir(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("staged directory: %w", err)
	}
	tag, path := "", ""
	for _, entry := range entries {
		if !entry.IsDir() || !isNewer(entry.Name(), u.CurrentVersion) {
			continue
		}
		candidate := filepath.Join(base, entry.Name(), u.binaryName())
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("staged candidate: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staged candidate is not a regular file: %s", candidate)
		}
		if tag == "" || isNewer(entry.Name(), tag) {
			tag, path = entry.Name(), candidate
		}
	}
	if tag == "" {
		return nil
	}
	dest, err := u.executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	// Temp copy next to the installed binary keeps Unix rename on one filesystem.
	temp, err := os.CreateTemp(filepath.Dir(dest), ".backscroll-update-*")
	if err != nil {
		return fmt.Errorf("replacement temporary file: %w", err)
	}
	tempPath := temp.Name()
	if err = temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	defer os.Remove(tempPath)
	input, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open staged candidate: %w", err)
	}
	err = hashfile.WriteAtomic(tempPath, input, 0755)
	closeErr := input.Close()
	if err != nil {
		return fmt.Errorf("copy staged candidate: %w", err)
	}
	if closeErr != nil {
		return closeErr
	}
	if err = u.replace(dest, tempPath); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	if err = u.exec(dest); err != nil {
		return fmt.Errorf("reexec: %w", err)
	}
	return nil
}
