package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	testBinaryDir  string
	testBinaryPath string
	testBinaryErr  error
	testBinaryOnce sync.Once
)

func TestMain(m *testing.M) {
	var err error
	testBinaryDir, err = os.MkdirTemp("", "backscroll-test-binary-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testBinaryPath = filepath.Join(testBinaryDir, "backscroll")
	if runtime.GOOS == "windows" {
		testBinaryPath += ".exe"
	}

	code := m.Run()
	if err := os.RemoveAll(testBinaryDir); err != nil && code == 0 {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}

func builtTestBinary(t *testing.T) string {
	t.Helper()
	testBinaryOnce.Do(func() {
		command := exec.Command("go", "build", "-mod=readonly", "-buildvcs=false", "-ldflags", "-X main.version=dev", "-o", testBinaryPath, ".")
		if output, err := command.CombinedOutput(); err != nil {
			testBinaryErr = fmt.Errorf("build shared Backscroll test binary: %w\n%s", err, output)
		}
	})
	if testBinaryErr != nil {
		t.Fatal(testBinaryErr)
	}
	return testBinaryPath
}
