//go:build darwin || linux

package skillinstall

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type noStatFileInfo struct{}

func (noStatFileInfo) Name() string       { return "missing-stat" }
func (noStatFileInfo) Size() int64        { return 0 }
func (noStatFileInfo) Mode() os.FileMode  { return 0o600 }
func (noStatFileInfo) ModTime() time.Time { return time.Time{} }
func (noStatFileInfo) IsDir() bool        { return false }
func (noStatFileInfo) Sys() any           { return nil }

func TestUnixPrivateFileAndLockValidation(t *testing.T) {
	if err := (*fileLock)(nil).close(); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "private")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateFile(file, 0o600); err == nil {
		t.Fatal("validatePrivateFile accepted an insecure mode")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := openExclusiveNoFollow(path, 0o600); err == nil {
		t.Fatal("openExclusiveNoFollow replaced an existing file")
	}
	link := filepath.Join(directory, "event-link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := openAppendNoFollow(link, 0o600); err == nil {
		t.Fatal("openAppendNoFollow followed a symbolic link")
	}
	appendPath := filepath.Join(directory, "insecure-events")
	if err := os.WriteFile(appendPath, []byte("event\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := openAppendNoFollow(appendPath, 0o600); err == nil {
		t.Fatal("openAppendNoFollow accepted an insecure existing file")
	}
	state := filepath.Join(directory, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireLock(state); err == nil {
		t.Fatal("acquireLock accepted an insecure lock file")
	}
	info := noStatFileInfo{}
	if _, err := linkCount(info); err == nil {
		t.Fatal("linkCount accepted metadata without POSIX stat data")
	}
	if _, err := ownerID(info); err == nil {
		t.Fatal("ownerID accepted metadata without POSIX stat data")
	}
	if _, err := deviceID(info); err == nil {
		t.Fatal("deviceID accepted metadata without POSIX stat data")
	}
	if err := syncDirectory(filepath.Join(directory, "missing")); err == nil {
		t.Fatal("syncDirectory accepted a missing directory")
	}
}

func TestPlanRejectsUnsafeFilesystemObjectsAndLayouts(t *testing.T) {
	t.Run("symlinked destination ancestor", func(t *testing.T) {
		home := testHome(t)
		outside := filepath.Join(filepath.Dir(home), "outside")
		if err := os.Mkdir(outside, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(home, ".agents")); err != nil {
			t.Fatal(err)
		}
		installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
		if _, err := installer.Plan(); err == nil || !strings.Contains(err.Error(), "unsafe ancestor") {
			t.Fatalf("Plan symlink ancestor error = %v", err)
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 0 {
			t.Fatalf("outside entries = %v, error = %v", entries, err)
		}
	})

	t.Run("symlinked state directory", func(t *testing.T) {
		home := testHome(t)
		outside := filepath.Join(filepath.Dir(home), "outside-state")
		if err := os.Mkdir(outside, 0o700); err != nil {
			t.Fatal(err)
		}
		state := filepath.Join(home, statePath)
		if err := os.MkdirAll(filepath.Dir(state), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, state); err != nil {
			t.Fatal(err)
		}
		installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
		if _, err := installer.Apply(""); err == nil {
			t.Fatal("Apply accepted a symlinked state directory")
		}
		entries, err := os.ReadDir(outside)
		if err != nil || len(entries) != 0 {
			t.Fatalf("outside state entries = %v, error = %v", entries, err)
		}
	})

	t.Run("overlapping destinations", func(t *testing.T) {
		home := testHome(t)
		installer, err := New(Options{
			Home: home, ConfigHome: filepath.Join(home, ".claude", "skills", "backscroll"),
			Shim: map[string][]byte{"SKILL.md": []byte("shim")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := installer.Plan(); err == nil || !strings.Contains(err.Error(), "overlapping destinations") {
			t.Fatalf("Plan overlap error = %v", err)
		}
	})

	t.Run("hard-linked file", func(t *testing.T) {
		home := testHome(t)
		installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
		target := targetPaths(installer.home, installer.configHome)[0]
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("unknown"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(target, target+"-second-link"); err != nil {
			t.Fatal(err)
		}
		if _, err := installer.Plan(); err == nil || !strings.Contains(err.Error(), "hard-linked file") {
			t.Fatalf("Plan hard-link error = %v", err)
		}
	})

	t.Run("special file", func(t *testing.T) {
		home := testHome(t)
		installer := testInstaller(t, home, dependencies{exchange: exchangePaths, moveNoReplace: moveNoReplace, syncDir: syncDirectory})
		target := targetPaths(installer.home, installer.configHome)[0]
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(target, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := installer.Plan(); err == nil || !strings.Contains(err.Error(), "special file") {
			t.Fatalf("Plan special-file error = %v", err)
		}
	})
}
