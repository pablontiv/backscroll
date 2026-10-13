package autoupdate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func stagedFixture(t *testing.T) (*Updater, string, string) {
	t.Helper()
	cache := t.TempDir()
	install := filepath.Join(t.TempDir(), "installed")
	if err := os.WriteFile(install, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	u := New("r", "b", Options{CacheDir: func() (string, error) { return cache, nil }, Executable: func() (string, error) { return install, nil }, Exec: func(string) error { return nil }})
	u.CurrentVersion = "v1.0.0"
	staged := filepath.Join(cache, "b", "staged", "v2.0.0", u.binaryName())
	if err := os.MkdirAll(filepath.Dir(staged), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}
	return u, staged, install
}
func TestApplySuccessAndCandidateRetention(t *testing.T) {
	u, staged, install := stagedFixture(t)
	called := false
	u.exec = func(path string) error {
		called = true
		if path != install {
			t.Fatal(path)
		}
		return nil
	}
	if err := u.ApplyStagedIfAvailable(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(install)
	if err != nil || string(data) != "new" || !called {
		t.Fatalf("data=%q err=%v called=%v", data, err, called)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("staged candidate lost: %v", err)
	}
}
func TestApplyErrorsRetainCandidate(t *testing.T) {
	expected := errors.New("failure")
	for _, phase := range []string{"cache", "scan", "executable", "temp", "copy", "replace", "exec", "candidate"} {
		t.Run(phase, func(t *testing.T) {
			u, staged, install := stagedFixture(t)
			switch phase {
			case "cache":
				u.cacheDir = func() (string, error) { return "", expected }
			case "scan":
				u.cacheDir = func() (string, error) { return install, nil }
			case "executable":
				u.executable = func() (string, error) { return "", expected }
			case "temp":
				u.executable = func() (string, error) { return filepath.Join(install, "not-dir"), nil }
			case "copy":
				u.executable = func() (string, error) {
					if err := os.Remove(staged); err != nil {
						t.Fatal(err)
					}
					return install, nil
				}
			case "replace":
				u.replace = func(string, string) error { return expected }
			case "exec":
				u.exec = func(string) error { return expected }
			case "candidate":
				if err := os.Remove(staged); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(staged, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := u.ApplyStagedIfAvailable(); err == nil {
				t.Fatal("error swallowed")
			}
			if phase != "copy" {
				if _, err := os.Stat(staged); err != nil {
					t.Fatalf("candidate lost: %v", err)
				}
			}
		})
	}
}
func TestApplyNormalAbsenceAndNewest(t *testing.T) {
	u, staged, install := stagedFixture(t)
	u.CurrentVersion = "v2.0.0"
	if err := u.ApplyStagedIfAvailable(); err != nil {
		t.Fatal(err)
	}
	u.CurrentVersion = "v1.0.0"
	newer := filepath.Join(filepath.Dir(filepath.Dir(staged)), "v3.0.0", u.binaryName())
	_ = os.MkdirAll(filepath.Dir(newer), 0755)
	_ = os.WriteFile(newer, []byte("newest"), 0755)
	if err := u.ApplyStagedIfAvailable(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(install)
	if string(data) != "newest" {
		t.Fatalf("installed %q", data)
	}
	u.cacheDir = func() (string, error) { return t.TempDir(), nil }
	if err := u.ApplyStagedIfAvailable(); err != nil {
		t.Fatal(err)
	}
}
