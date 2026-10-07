//go:build unix

package main

import (
	"bytes"
	"io"
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
	ready := os.NewFile(3, "signal-ready")
	if ready == nil {
		os.Exit(2)
	}
	code := realMainWithDependencies(os.Stdout, os.Stderr, []string{"rebuild"}, processDependencies{
		notifySignals: newProcessSignalNotifier(ready),
	})
	_ = ready.Close()
	os.Exit(code)
}

func TestRealMainSIGTERMExitsPromptlyWithoutDuplicateDiagnostic(t *testing.T) {
	dbPath, cleanup := testEnv(t)
	defer cleanup()

	// Hold the startup lease so the child cannot finish between announcing that
	// its signal handler is ready and receiving SIGTERM.
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

	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readyReader.Close()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessSignalHelper$")
	cmd.Env = append(os.Environ(), processSignalHelperEnv+"=1")
	cmd.ExtraFiles = []*os.File{readyWriter}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = readyWriter.Close()
		t.Fatal(err)
	}
	if err := readyWriter.Close(); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ready := make(chan error, 1)
	go func() {
		var announced [1]byte
		_, err := io.ReadFull(readyReader, announced[:])
		ready <- err
	}()

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("wait for signal readiness: %v", err)
		}
	case err := <-done:
		t.Fatalf("child exited before signal readiness: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("child did not announce signal readiness")
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
	if !strings.Contains(gotStderr, "canceled") {
		t.Fatalf("stderr does not report cancellation: %q", gotStderr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}
