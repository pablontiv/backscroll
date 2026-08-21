package startuplock

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func init() {
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
	go func() {
		for {
			runtime.Gosched()
		}
	}()
	select {}
}

func TestLockHelperProcess(t *testing.T) {}

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

	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("helper readiness line=%q err=%v", line, err)
	}

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

	if line, err := bufio.NewReader(holdStdout).ReadString('\n'); err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("holder readiness line=%q err=%v", line, err)
	}

	tryCmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	tryCmd.Env = append(os.Environ(),
		"BACKSCROLL_LOCK_HELPER=try",
		"BACKSCROLL_LOCK_DB="+dbPath,
	)
	out, err := tryCmd.Output()
	if err != nil {
		t.Fatalf("try helper failed: %v", err)
	}
	if got := string(out); got != "busy\n" {
		t.Fatalf("output=%q want busy\\n", got)
	}
}
