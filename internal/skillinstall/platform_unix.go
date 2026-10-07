//go:build darwin || linux

package skillinstall

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func currentUserID() int {
	return os.Getuid()
}

type fileLock struct {
	file *os.File
}

func acquireLock(state string) (*fileLock, error) {
	if err := makePrivateDir(state); err != nil {
		return nil, err
	}
	path := state + string(os.PathSeparator) + "lock"
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open installation lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if err := validatePrivateFile(file, 0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("unsafe lock file: %w", err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock skill installation: %w", err)
	}
	return &fileLock{file: file}, nil
}

func (lock *fileLock) close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	fd := int(lock.file.Fd())
	return errors.Join(unix.Flock(fd, unix.LOCK_UN), lock.file.Close())
}

func openExclusiveNoFollow(path string, mode os.FileMode) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if err := validatePrivateFile(file, mode); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func openAppendNoFollow(path string, mode os.FileMode) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if err := validatePrivateFile(file, mode); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func validatePrivateFile(file *os.File, mode os.FileMode) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	links, err := linkCount(info)
	if err != nil {
		return err
	}
	uid, err := ownerID(info)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || links != 1 || uid != os.Getuid() || info.Mode().Perm() != mode.Perm() {
		return errors.New("file must be regular, singly linked, owned by you, and have secure permissions")
	}
	return nil
}

func linkCount(info os.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("file metadata does not contain POSIX stat data")
	}
	return uint64(stat.Nlink), nil
}

func ownerID(info os.FileInfo) (int, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("file metadata does not contain POSIX stat data")
	}
	return int(stat.Uid), nil
}

func deviceID(info os.FileInfo) (uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, errors.New("file metadata does not contain POSIX stat data")
	}
	return uint64(stat.Dev), nil
}
