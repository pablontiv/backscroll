//go:build !darwin && !linux

package skillinstall

import "errors"

type pinnedDir struct{}

func openPinnedDir(string) (*pinnedDir, error) {
	return nil, errors.New("skill installation is supported only on macOS and Linux")
}

func (*pinnedDir) close() error { return nil }
func (*pinnedDir) stablePath() error {
	return errors.New("skill installation is supported only on macOS and Linux")
}
func (*pinnedDir) sync() error {
	return errors.New("skill installation is supported only on macOS and Linux")
}
func (*pinnedDir) snapshot(string) (Snapshot, error) {
	return Snapshot{}, errors.New("skill installation is supported only on macOS and Linux")
}
func (*pinnedDir) symlink(string, string) error {
	return errors.New("skill installation is supported only on macOS and Linux")
}
func (*pinnedDir) remove(string) error {
	return errors.New("skill installation is supported only on macOS and Linux")
}
