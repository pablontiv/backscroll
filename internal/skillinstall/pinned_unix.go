//go:build darwin || linux

package skillinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

// pinnedDir keeps one verified directory open while a transaction inspects and
// changes entries in that directory.
type pinnedDir struct {
	fd   int
	path string
}

func openPinnedDir(path string) (*pinnedDir, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("pinned directory must be absolute: %s", path)
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	for _, part := range parts {
		if part == "" {
			continue
		}
		next, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, fmt.Errorf("open pinned directory %s: %w", path, openErr)
		}
		fd = next
	}
	return &pinnedDir{fd: fd, path: path}, nil
}

func (directory *pinnedDir) close() error {
	if directory == nil || directory.fd < 0 {
		return nil
	}
	err := unix.Close(directory.fd)
	directory.fd = -1
	return err
}

func (directory *pinnedDir) stablePath() error {
	current, err := openPinnedDir(directory.path)
	if err != nil {
		return fmt.Errorf("pinned directory path changed: %s: %w", directory.path, err)
	}
	defer current.close()
	var heldStat unix.Stat_t
	var currentStat unix.Stat_t
	if err := unix.Fstat(directory.fd, &heldStat); err != nil {
		return err
	}
	if err := unix.Fstat(current.fd, &currentStat); err != nil {
		return err
	}
	if heldStat.Dev != currentStat.Dev || heldStat.Ino != currentStat.Ino {
		return fmt.Errorf("pinned directory path changed: %s", directory.path)
	}
	return nil
}

func (directory *pinnedDir) sync() error {
	if err := unix.Fsync(directory.fd); err != nil {
		return fmt.Errorf("fsync directory %s: %w", directory.path, err)
	}
	return nil
}

func (directory *pinnedDir) snapshot(name string) (Snapshot, error) {
	if !validEntryName(name) {
		return Snapshot{}, fmt.Errorf("invalid pinned entry name: %q", name)
	}
	return snapshotAt(directory.fd, name)
}

func (directory *pinnedDir) symlink(target, name string) error {
	if !validEntryName(name) {
		return fmt.Errorf("invalid pinned entry name: %q", name)
	}
	return unix.Symlinkat(target, directory.fd, name)
}

func (directory *pinnedDir) remove(name string) error {
	if !validEntryName(name) {
		return fmt.Errorf("invalid pinned entry name: %q", name)
	}
	return unix.Unlinkat(directory.fd, name, 0)
}

func validEntryName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name
}

func snapshotAt(parentFD int, name string) (Snapshot, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return Snapshot{Kind: "absent"}, nil
		}
		return Snapshot{}, err
	}
	mode := uint32(stat.Mode & 0o7777)
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFLNK:
		target, err := readlinkAt(parentFD, name)
		if err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Kind: "link", Target: target}, nil
	case unix.S_IFREG:
		fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return Snapshot{}, err
		}
		file := os.NewFile(uintptr(fd), name)
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return Snapshot{}, err
		}
		links, err := linkCount(info)
		if err != nil {
			return Snapshot{}, err
		}
		if links != 1 {
			return Snapshot{}, fmt.Errorf("hard-linked file not supported: %s", name)
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, file); err != nil {
			return Snapshot{}, err
		}
		return Snapshot{Kind: "file", Mode: mode, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
	case unix.S_IFDIR:
		fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return Snapshot{}, err
		}
		file := os.NewFile(uintptr(fd), name)
		entries, readErr := file.ReadDir(-1)
		if readErr != nil {
			_ = file.Close()
			return Snapshot{}, readErr
		}
		sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
		result := Snapshot{Kind: "directory", Mode: mode, Entries: make(map[string]Snapshot, len(entries))}
		for _, entry := range entries {
			child, err := snapshotAt(fd, entry.Name())
			if err != nil {
				_ = file.Close()
				return Snapshot{}, err
			}
			result.Entries[entry.Name()] = child
		}
		if err := file.Close(); err != nil {
			return Snapshot{}, err
		}
		return result, nil
	default:
		return Snapshot{}, fmt.Errorf("special file not supported: %s", name)
	}
}

func readlinkAt(parentFD int, name string) (string, error) {
	for size := 128; size <= 1<<20; size *= 2 {
		buffer := make([]byte, size)
		count, err := unix.Readlinkat(parentFD, name, buffer)
		if err != nil {
			return "", err
		}
		if count < len(buffer) {
			return string(buffer[:count]), nil
		}
	}
	return "", errors.New("symbolic link target is too long")
}
