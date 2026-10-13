package autoupdate

import (
	"errors"
	"fmt"
	"os"
)

type windowsReplaceOps struct {
	rename func(string, string) error
	remove func(string) error
}

// Cleanup after installation is best-effort: the running Windows image may
// keep its backup mapped. A cleanup failure must never suppress reexecution.
// The next process retries cleanup via cleanupBackup before scanning candidates.
func replaceWindows(dest, src string, ops windowsReplaceOps) error {
	backup := dest + ".old"
	if _, err := os.Lstat(backup); err == nil {
		return fmt.Errorf("old executable backup remains: %s", backup)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := ops.rename(dest, backup); err != nil {
		return err
	}
	if err := ops.rename(src, dest); err != nil {
		return errors.Join(err, ops.rename(backup, dest))
	}
	_ = ops.remove(backup)
	return nil
}
