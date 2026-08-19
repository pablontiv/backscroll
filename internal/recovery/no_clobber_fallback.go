//go:build !darwin && !linux

package recovery

func noClobberMove(oldPath, newPath string) error {
	return noClobberUnsupportedError{oldPath: oldPath, newPath: newPath}
}
