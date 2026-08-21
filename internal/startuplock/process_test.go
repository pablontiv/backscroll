package startuplock

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	// The helper is env-gated so the same test binary can act as either the
	// parent test or the child process that owns/contends for the lock.
	mode := os.Getenv("BACKSCROLL_LOCK_HELPER")
	if mode != "hold" && mode != "try" {
		return
	}

	dbPath := os.Getenv("BACKSCROLL_LOCK_DB")
	lease, locked, err := TryAcquire(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "acquire locked=%v err=%v\n", locked, err)
		os.Exit(2)
	}
	if mode == "try" {
		// The decoy process exits immediately after reporting contention so the
		// parent can prove cross-process busy handling without waiting forever.
		if locked {
			_ = lease.Release()
			fmt.Fprintln(os.Stdout, "acquired")
		} else {
			fmt.Fprintln(os.Stdout, "busy")
		}
		os.Exit(0)
	}
	if !locked {
		fmt.Fprintf(os.Stderr, "acquire locked=%v err=%v\n", locked, err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "locked")
	_ = os.Stdout.Sync()
	// Keep the helper alive without spinning so the test process stays parked
	// until the parent kills it.
	select {
	case <-time.After(24 * time.Hour):
	}
}

func TestLockHelperProcess(t *testing.T) {}

func waitForReadyLine(t *testing.T, r io.Reader, name string) {
	t.Helper()
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()
	select {
	case line := <-lineCh:
		if strings.TrimSpace(line) != "locked" {
			t.Fatalf("%s readiness line=%q err=<nil>", name, line)
		}
	case err := <-errCh:
		t.Fatalf("%s readiness line=<nil> err=%v", name, err)
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for %s readiness", name)
	}
}

func TestProcessDeathReleasesLock(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	cmd.Env = append(os.Environ(),
		"BACKSCROLL_LOCK_HELPER=hold",
		"BACKSCROLL_LOCK_DB="+dbPath,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	waitForReadyLine(t, stdout, "helper")

	if lease, locked, err := TryAcquire(dbPath); err != nil || locked || lease != nil {
		t.Fatalf("parent bypassed helper lock: lease=%v locked=%v err=%v", lease, locked, err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for {
		lease, locked, err := TryAcquire(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if locked {
			if err := lease.Release(); err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lock remained held after helper process death")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestProcessTryReportsBusyFromSecondProcess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.db")
	holdCmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	holdCmd.Env = append(os.Environ(),
		"BACKSCROLL_LOCK_HELPER=hold",
		"BACKSCROLL_LOCK_DB="+dbPath,
	)
	holdStdout, err := holdCmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	holdCmd.Stderr = os.Stderr
	if err := holdCmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holdCmd.Process.Kill(); _ = holdCmd.Wait() }()

	waitForReadyLine(t, holdStdout, "holder")

	tryCmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	tryCmd.Env = append(os.Environ(),
		"BACKSCROLL_LOCK_HELPER=try",
		"BACKSCROLL_LOCK_DB="+dbPath,
	)
	outCh := make(chan []byte, 1)
	errCh2 := make(chan error, 1)
	go func() {
		out, err := tryCmd.Output()
		if err != nil {
			errCh2 <- err
			return
		}
		outCh <- out
	}()
	select {
	case out := <-outCh:
		if got := string(out); got != "busy\n" {
			t.Fatalf("output=%q want busy\\n", got)
		}
	case err := <-errCh2:
		t.Fatalf("try helper failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for try helper")
	}
}
