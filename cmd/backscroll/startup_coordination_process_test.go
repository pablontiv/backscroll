package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/config"
	"github.com/pablontiv/backscroll/internal/startuplock"
	"github.com/pablontiv/backscroll/internal/storage"
)

const (
	startupCoordinationSeedText = "singleflight snapshot sentinel"
	startupCoordinationSeedUUID = "seed-u"
)

func TestStartupCoordinationHelperProcess(t *testing.T) {
	if os.Getenv("BACKSCROLL_COORDINATION_HELPER") != "1" {
		return
	}

	counter := os.Getenv("BACKSCROLL_SYNC_COUNTER")
	ready := os.Getenv("BACKSCROLL_SYNC_READY")
	release := os.Getenv("BACKSCROLL_SYNC_RELEASE")
	block := os.Getenv("BACKSCROLL_SYNC_BLOCK") == "1"
	if wait := os.Getenv("BACKSCROLL_MUTATION_WAIT"); wait != "" {
		parsed, err := time.ParseDuration(wait)
		if err != nil {
			t.Fatal(err)
		}
		startupMutationWait = parsed
	}

	startupSync = func(*config.Config, io.Writer) error {
		if counter == "" {
			return fmt.Errorf("BACKSCROLL_SYNC_COUNTER is required when startup sync runs")
		}
		file, err := os.OpenFile(counter, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		if block {
			if ready == "" || release == "" {
				return fmt.Errorf("BACKSCROLL_SYNC_READY and BACKSCROLL_SYNC_RELEASE are required when sync blocks")
			}
			if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil {
				return err
			}
			deadline := time.NewTimer(30 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-deadline.C:
					return fmt.Errorf("timed out waiting for BACKSCROLL_SYNC_RELEASE")
				case <-ticker.C:
					if _, err := os.Stat(release); err == nil {
						return nil
					} else if !os.IsNotExist(err) {
						return err
					}
				}
			}
		}
		return nil
	}

	argvJSON := os.Getenv("BACKSCROLL_HELPER_ARGV")
	var argv []string
	if err := json.Unmarshal([]byte(argvJSON), &argv); err != nil {
		t.Fatal(err)
	}
	root := buildRootCmd(os.Stdout, os.Stderr)
	root.SetArgs(argv)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestStartupCoordinationSingleFlightSnapshotFollowers(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	seedStartupCoordinationDB(t, dbPath)
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	counter := filepath.Join(dir, "sync-counter.txt")
	ready := filepath.Join(dir, "owner-ready")
	release := filepath.Join(dir, "owner-release")
	owner := startCoordinationChild(t, []string{"status", "--json"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
		"BACKSCROLL_SYNC_READY="+ready,
		"BACKSCROLL_SYNC_RELEASE="+release,
		"BACKSCROLL_SYNC_BLOCK=1",
	)
	waitForPath(t, ready, 2*time.Second)

	search := startCoordinationChild(t, []string{"search", startupCoordinationSeedText, "--all-projects", "--json"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
	)
	status := startCoordinationChild(t, []string{"status", "--json"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
	)
	requireChildSuccess(t, search, 2*time.Second)
	requireChildSuccess(t, status, 2*time.Second)

	assertSearchJSONHasSeed(t, search.stdout.String())
	assertStatusJSONHasSeed(t, status.stdout.String())
	assertStderrContains(t, search, "warning: sync_in_progress:")
	assertStderrContains(t, status, "warning: sync_in_progress:")
	assertCounterLines(t, counter, 1)

	writeFile(t, release, "release")
	requireChildSuccess(t, owner, 2*time.Second)
	assertStartupLockSidecarExists(t, dbPath)
}

func TestStartupCoordinationMutationTimeoutNoSideEffect(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	seedStartupCoordinationDB(t, dbPath)
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	counter := filepath.Join(dir, "sync-counter.txt")
	ready := filepath.Join(dir, "owner-ready")
	release := filepath.Join(dir, "owner-release")
	owner := startCoordinationChild(t, []string{"status", "--json"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
		"BACKSCROLL_SYNC_READY="+ready,
		"BACKSCROLL_SYNC_RELEASE="+release,
		"BACKSCROLL_SYNC_BLOCK=1",
	)
	waitForPath(t, ready, 2*time.Second)

	annotate := startCoordinationChild(t, []string{"annotate", "--uuid", startupCoordinationSeedUUID, "--kind", "correction", "--label", "should-not-write"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
		"BACKSCROLL_MUTATION_WAIT=150ms",
	)
	err := waitForChild(t, annotate, 2*time.Second)
	if err == nil {
		t.Fatalf("annotate unexpectedly succeeded; stdout=%q stderr=%q", annotate.stdout.String(), annotate.stderr.String())
	}
	if isChildTimeout(err) {
		t.Fatalf("annotate did not exit within deadline: %v stdout=%q stderr=%q", err, annotate.stdout.String(), annotate.stderr.String())
	}
	combined := annotate.stdout.String() + annotate.stderr.String()
	if !strings.Contains(combined, "sync_in_progress") || !strings.Contains(combined, "retry the command") {
		t.Fatalf("annotate diagnostic missing sync_in_progress retry guidance; stdout=%q stderr=%q", annotate.stdout.String(), annotate.stderr.String())
	}
	assertCounterLines(t, counter, 1)
	assertNoAnnotationLabelReadOnly(t, dbPath, "should-not-write")

	writeFile(t, release, "release")
	requireChildSuccess(t, owner, 2*time.Second)
	assertStartupLockSidecarExists(t, dbPath)
}

func TestStartupCoordinationCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "index.db")
	seedStartupCoordinationDB(t, dbPath)
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	counter := filepath.Join(dir, "sync-counter.txt")
	ready := filepath.Join(dir, "owner-ready")
	owner := startCoordinationChild(t, []string{"status", "--json"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
		"BACKSCROLL_SYNC_READY="+ready,
		"BACKSCROLL_SYNC_RELEASE="+filepath.Join(dir, "never-release"),
		"BACKSCROLL_SYNC_BLOCK=1",
	)
	waitForPath(t, ready, 2*time.Second)
	killChildAndWait(t, owner, 2*time.Second)
	assertCounterLines(t, counter, 1)

	replacement := startCoordinationChild(t, []string{"status", "--json"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
	)
	requireChildSuccess(t, replacement, 5*time.Second)
	assertStatusJSONHasSeed(t, replacement.stdout.String())
	assertCounterLines(t, counter, 2)
	if strings.Contains(replacement.stderr.String(), "sync_in_progress") {
		t.Fatalf("replacement owner emitted stale sync warning: stderr=%q", replacement.stderr.String())
	}
	assertStartupLockSidecarExists(t, dbPath)
}

func TestStartupCoordinationMetadataFollowerMissingDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "missing.db")
	setIndexPolicyEnv(t, dbPath, t.TempDir())

	lease, acquired, err := startuplock.TryAcquire(dbPath)
	if err != nil {
		t.Fatalf("acquire parent startup lock: %v", err)
	}
	if !acquired {
		t.Fatal("parent did not acquire startup lock for missing DB")
	}
	t.Cleanup(func() {
		if err := lease.Release(); err != nil {
			t.Errorf("release parent startup lock: %v", err)
		}
	})

	counter := filepath.Join(dir, "sync-counter.txt")
	configChild := startCoordinationChild(t, []string{"config", "--json"},
		"BACKSCROLL_SYNC_COUNTER="+counter,
	)
	requireChildSuccess(t, configChild, 2*time.Second)
	assertConfigJSONPath(t, configChild.stdout.String(), dbPath)
	assertStderrContains(t, configChild, "warning: sync_in_progress: startup sync active; continuing without a new startup sync")
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("database path exists after metadata follower; stat err=%v", err)
	}
	assertCounterLines(t, counter, 0)
	assertStartupLockSidecarExists(t, dbPath)
}

type coordinationChild struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
	done   chan struct{}
	mu     sync.Mutex
	err    error
}

type childTimeoutError struct {
	timeout time.Duration
}

func (e childTimeoutError) Error() string {
	return fmt.Sprintf("child timed out after %s", e.timeout)
}

func isChildTimeout(err error) bool {
	_, ok := err.(childTimeoutError)
	return ok
}

func startCoordinationChild(t *testing.T, argv []string, extraEnv ...string) *coordinationChild {
	t.Helper()
	encoded, err := json.Marshal(argv)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupCoordinationHelperProcess$")
	cmd.Env = append(os.Environ(),
		"BACKSCROLL_COORDINATION_HELPER=1",
		"BACKSCROLL_HELPER_ARGV="+string(encoded),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	child := &coordinationChild{cmd: cmd, done: make(chan struct{})}
	cmd.Stdout = &child.stdout
	cmd.Stderr = &child.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child %v: %v", argv, err)
	}
	go func() {
		err := cmd.Wait()
		child.mu.Lock()
		child.err = err
		child.mu.Unlock()
		close(child.done)
	}()
	t.Cleanup(func() {
		select {
		case <-child.done:
			return
		default:
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-child.done:
		case <-time.After(2 * time.Second):
			t.Errorf("child cleanup wait timed out for %v", argv)
		}
	})
	return child
}

func waitForChild(t *testing.T, child *coordinationChild, timeout time.Duration) error {
	t.Helper()
	select {
	case <-child.done:
		return child.waitErr()
	case <-time.After(timeout):
		if child.cmd.Process != nil {
			_ = child.cmd.Process.Kill()
		}
		select {
		case <-child.done:
		case <-time.After(2 * time.Second):
		}
		return childTimeoutError{timeout: timeout}
	}
}

func requireChildSuccess(t *testing.T, child *coordinationChild, timeout time.Duration) {
	t.Helper()
	if err := waitForChild(t, child, timeout); err != nil {
		t.Fatalf("child failed: %v stdout=%q stderr=%q", err, child.stdout.String(), child.stderr.String())
	}
}

func killChildAndWait(t *testing.T, child *coordinationChild, timeout time.Duration) {
	t.Helper()
	select {
	case <-child.done:
		return
	default:
	}
	if child.cmd.Process != nil {
		if err := child.cmd.Process.Kill(); err != nil {
			t.Fatalf("kill child: %v", err)
		}
	}
	select {
	case <-child.done:
	case <-time.After(timeout):
		t.Fatalf("killed child did not exit within %s stdout=%q stderr=%q", timeout, child.stdout.String(), child.stderr.String())
	}
}

func (c *coordinationChild) waitErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func waitForPath(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", path)
		case <-ticker.C:
			if _, err := os.Stat(path); err == nil {
				return
			} else if !os.IsNotExist(err) {
				t.Fatalf("stat %s: %v", path, err)
			}
		}
	}
}

func seedStartupCoordinationDB(t *testing.T, dbPath string) {
	t.Helper()
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	files := []storage.IndexedFile{{
		SourcePath: "/startup-coordination/session.jsonl",
		Source:     "session",
		Hash:       "startup-coordination-hash",
		Project:    "startup-coordination-project",
		Messages: []storage.IndexedMessage{{
			Ordinal:           0,
			Role:              "user",
			Text:              startupCoordinationSeedText,
			UUID:              startupCoordinationSeedUUID,
			Timestamp:         "2026-01-01T00:00:00Z",
			ContentType:       "text",
			ExtractionVersion: storage.CurrentExtractionVersion,
		}},
		Tags: []string{"startup-coordination"},
	}}
	if err := db.SyncFiles(files); err != nil {
		_ = db.Close()
		t.Fatalf("seed sync: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}
}

func assertSearchJSONHasSeed(t *testing.T, stdout string) {
	t.Helper()
	var rows []struct {
		SourcePath string `json:"source_path"`
		Snippet    string `json:"snippet"`
		Role       string `json:"role"`
	}
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("search stdout is not clean JSON: %v stdout=%q", err, stdout)
	}
	for _, row := range rows {
		if row.SourcePath == "/startup-coordination/session.jsonl" && row.Role == "user" && strings.Contains(row.Snippet, "singleflight") {
			return
		}
	}
	t.Fatalf("search JSON missing seeded row: %+v", rows)
}

func assertStatusJSONHasSeed(t *testing.T, stdout string) {
	t.Helper()
	var payload struct {
		Index struct {
			TotalFiles    int `json:"total_files"`
			TotalMessages int `json:"total_messages"`
		} `json:"index"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("status stdout is not clean JSON: %v stdout=%q", err, stdout)
	}
	if payload.Index.TotalFiles != 1 || payload.Index.TotalMessages != 1 {
		t.Fatalf("status index counts=%+v, want one seeded file/message", payload.Index)
	}
}

func assertConfigJSONPath(t *testing.T, stdout, dbPath string) {
	t.Helper()
	var payload struct {
		Database struct {
			Path string `json:"path"`
		} `json:"database"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("config stdout is not clean JSON: %v stdout=%q", err, stdout)
	}
	if payload.Database.Path != dbPath {
		t.Fatalf("config database path=%q, want %q", payload.Database.Path, dbPath)
	}
}

func assertStderrContains(t *testing.T, child *coordinationChild, want string) {
	t.Helper()
	if !strings.Contains(child.stderr.String(), want) {
		t.Fatalf("stderr missing %q: stdout=%q stderr=%q", want, child.stdout.String(), child.stderr.String())
	}
}

func assertCounterLines(t *testing.T, counter string, want int) {
	t.Helper()
	lines := readCounterLines(t, counter)
	if len(lines) != want {
		t.Fatalf("sync counter lines=%d (%v), want %d", len(lines), lines, want)
	}
}

func readCounterLines(t *testing.T, counter string) []string {
	t.Helper()
	data, err := os.ReadFile(counter)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read sync counter: %v", err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func assertNoAnnotationLabelReadOnly(t *testing.T, dbPath, label string) {
	t.Helper()
	db, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open db read-only: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close read-only db: %v", err)
		}
	}()
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM annotations WHERE item_uuid = ? AND kind = ? AND label = ?`, startupCoordinationSeedUUID, "correction", label).Scan(&count); err != nil {
		t.Fatalf("query annotations read-only: %v", err)
	}
	if count != 0 {
		t.Fatalf("annotation label %q rows=%d, want zero", label, count)
	}
}

func assertStartupLockSidecarExists(t *testing.T, dbPath string) {
	t.Helper()
	path := dbPath + ".startup-sync.lock"
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("startup lock sidecar %s missing: %v", path, err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("startup lock sidecar %s mode=%s, want regular file", path, info.Mode())
	}
}
