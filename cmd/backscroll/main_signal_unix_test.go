//go:build unix

package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/startuplock"
)

const processSignalHelperEnv = "BACKSCROLL_PROCESS_SIGNAL_HELPER"

func TestProcessSignalHelper(t *testing.T) {
	if os.Getenv(processSignalHelperEnv) != "1" {
		return
	}
	os.Exit(realMain(os.Stdout, os.Stderr, []string{"rebuild"}))
}

func TestRealMainSIGTERMExitsPromptlyWithoutDuplicateDiagnostic(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	// Hold the startup lease so the child remains deterministically in the
	// context-aware acquisition path until SIGTERM cancels it.
	lease, acquired, err := startuplock.TryAcquire(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("failed to acquire parent startup lease")
	}
	defer func() {
		if err := lease.Release(); err != nil {
			t.Errorf("release parent startup lease: %v", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessSignalHelper$")
	cmd.Env = append(os.Environ(), processSignalHelperEnv+"=1")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// The held lease keeps the command alive. This bounded readiness window also
	// ensures realMain has installed its signal context before the signal is sent.
	select {
	case err := <-done:
		t.Fatalf("child exited before signal: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	case <-time.After(200 * time.Millisecond):
	}

	sentAt := time.Now()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled child exited successfully")
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 1 {
			t.Fatalf("child exit=%v want exit code 1", err)
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("child did not exit promptly after SIGTERM")
	}
	if elapsed := time.Since(sentAt); elapsed >= 2*time.Second {
		t.Fatalf("signal exit took %s", elapsed)
	}

	gotStderr := stderr.String()
	if count := strings.Count(gotStderr, "diagnostic:"); count != 1 {
		t.Fatalf("diagnostic count=%d want 1; stderr=%q", count, gotStderr)
	}
	if !strings.Contains(gotStderr, "startup lock wait canceled") {
		t.Fatalf("stderr does not report cancellation: %q", gotStderr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}
