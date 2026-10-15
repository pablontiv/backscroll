// Package presets installs the input presets shipped with Backscroll.
package presets

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pablontiv/backscroll/internal/input_config"
)

const fileMode = 0o600

var presetNames = []string{
	"claude.inputs.toml",
	"pi.inputs.toml",
	"omp.inputs.toml",
	"decisions.inputs.toml",
	"opencode.inputs.toml",
	"codex.inputs.toml",
}

//go:embed assets/*.inputs.toml
var presetFiles embed.FS

// Options controls preset installation.
type Options struct {
	Overwrite  bool
	ConfigHome string
}

// Result reports the presets that Install changed or preserved.
type Result struct {
	Installed []string `json:"installed"`
	Skipped   []string `json:"skipped"`
}

// Install writes the shipped presets to the resolved Backscroll inputs directory.
func Install(options Options) (Result, error) {
	dir, err := inputsDir(options.ConfigHome)
	if err != nil {
		return Result{}, err
	}
	if err := safeParents(dir); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Result{}, fmt.Errorf("create inputs directory %s: %w", dir, err)
	}
	if err := safeParents(filepath.Join(dir, "placeholder")); err != nil {
		return Result{}, err
	}

	var result Result
	for _, name := range presetNames {
		installed, err := installOne(dir, name, options.Overwrite)
		if err != nil {
			return result, err
		}
		if installed {
			result.Installed = append(result.Installed, name)
		} else {
			result.Skipped = append(result.Skipped, name)
		}
	}
	return result, nil
}

func inputsDir(configHome string) (string, error) {
	if configHome == "" {
		dir, err := input_config.InputsDir()
		if err != nil {
			return "", fmt.Errorf("resolve inputs directory: %w", err)
		}
		return canonicalDarwinPath(dir)
	}
	if !filepath.IsAbs(configHome) {
		return "", errors.New("config home must be absolute")
	}
	return canonicalDarwinPath(filepath.Join(filepath.Clean(configHome), "backscroll", "inputs"))
}

func canonicalDarwinPath(path string) (string, error) {
	if runtime.GOOS != "darwin" || path != "/var" && !strings.HasPrefix(path, "/var/") {
		return path, nil
	}
	resolved, err := filepath.EvalSymlinks("/var")
	if err != nil {
		return "", err
	}
	if path == "/var" {
		return resolved, nil
	}
	return filepath.Join(resolved, strings.TrimPrefix(path, "/var/")), nil
}

func installOne(dir, name string, overwrite bool) (bool, error) {
	destination := filepath.Join(dir, name)
	if !overwrite {
		_, err := os.Lstat(destination)
		switch {
		case err == nil:
			return false, nil
		case !errors.Is(err, os.ErrNotExist):
			return false, fmt.Errorf("inspect preset %s: %w", destination, err)
		}
	}

	data, err := presetFiles.ReadFile(filepath.ToSlash(filepath.Join("assets", name)))
	if err != nil {
		return false, fmt.Errorf("read shipped preset %s: %w", name, err)
	}

	temporary, err := writeTemporary(dir, name, data)
	if err != nil {
		return false, fmt.Errorf("prepare preset %s: %w", destination, err)
	}
	defer func() { _ = os.Remove(temporary) }()

	if overwrite {
		if err := os.Rename(temporary, destination); err != nil {
			return false, fmt.Errorf("replace preset %s: %w", destination, err)
		}
		if err := syncDirectory(dir); err != nil {
			return false, fmt.Errorf("sync preset directory: %w", err)
		}
		return true, nil
	}

	if err := os.Link(temporary, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("install preset %s: %w", destination, err)
	}
	if err := os.Remove(temporary); err != nil {
		return false, fmt.Errorf("remove temporary preset %s: %w", temporary, err)
	}
	temporary = ""
	if err := syncDirectory(dir); err != nil {
		return false, fmt.Errorf("sync preset directory: %w", err)
	}
	return true, nil
}

func safeParents(path string) error {
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("unsafe preset ancestor: %s", parent)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if filepath.Dir(parent) == parent {
			return nil
		}
	}
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func writeTemporary(dir, name string, data []byte) (path string, err error) {
	file, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return "", err
	}
	path = file.Name()
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()

	if err = file.Chmod(fileMode); err != nil {
		return path, err
	}
	if _, err = file.Write(data); err != nil {
		return path, err
	}
	if err = file.Sync(); err != nil {
		return path, err
	}
	return path, nil
}
