package autoupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPublicationFailuresAndCancellation(t *testing.T) {
	for _, phase := range []string{"write", "sync", "close", "rename", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "binary")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New(phase)
			ops := publicationOps{}
			switch phase {
			case "write":
				ops.write = func(*os.File, []byte) (int, error) { return 0, failure }
			case "sync":
				ops.sync = func(*os.File) error { return failure }
			case "close":
				ops.close = func(f *os.File) error { _ = f.Close(); return failure }
			case "rename":
				ops.rename = func(string, string) error { return failure }
			case "cancel":
				ops.close = func(f *os.File) error { err := f.Close(); cancel(); return err }
			}
			if err := writeAtomicWith(ctx, dest, strings.NewReader("complete"), 0755, ops); err == nil {
				t.Fatal("failure swallowed")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatalf("publication or temp survived failure: %v", entries)
			}
		})
	}
}
func TestLimitsBoundaryAndReaderCancellation(t *testing.T) {
	for _, n := range []int{3, 4} {
		data, err := readBounded(context.Background(), strings.NewReader(strings.Repeat("x", n)), 3, "test")
		if n == 3 && (err != nil || len(data) != 3) {
			t.Fatal(err)
		}
		if n == 4 && (err == nil || !strings.Contains(err.Error(), "exceeds byte limit")) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readBounded(ctx, strings.NewReader("x"), 10, "test"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	u := New("r", "b")
	dest := filepath.Join(t.TempDir(), "binary")
	if err := u.extractTarContext(ctx, []byte("bad"), dest); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := u.extractZipContext(ctx, []byte("bad"), dest); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestSymlinkCandidatesAndPredictableTempUntouched(t *testing.T) {
	u, staged, _ := stagedFixture(t)
	target := filepath.Join(t.TempDir(), "target")
	_ = os.WriteFile(target, []byte("safe"), 0600)
	_ = os.Remove(staged)
	if err := os.Symlink(target, staged); err != nil {
		t.Skip(err)
	}
	if err := u.ApplyStagedIfAvailable(); err == nil {
		t.Fatal("symlink candidate accepted")
	}
	dir := t.TempDir()
	dest := filepath.Join(dir, "binary")
	_ = os.Symlink(target, dest+".tmp")
	if err := writeAtomic(context.Background(), dest, strings.NewReader("complete"), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "safe" {
		t.Fatal("predictable temp symlink followed")
	}
	if err := writeAtomic(context.Background(), staged, strings.NewReader("bad"), 0755); err == nil {
		t.Fatal("symlink publication accepted")
	}
}
func TestWindowsCleanupDoesNotSuppressReexec(t *testing.T) {
	u, staged, install := stagedFixture(t)
	var calls []string
	ops := windowsReplaceOps{
		rename: func(a, b string) error { calls = append(calls, "rename"); return os.Rename(a, b) },
		remove: func(string) error { calls = append(calls, "cleanup"); return errors.New("backup mapped") },
	}
	u.replace = func(dest, src string) error { return replaceWindows(dest, src, ops) }
	u.exec = func(string) error { calls = append(calls, "exec"); return nil }
	if err := u.ApplyStagedIfAvailable(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "rename,rename,cleanup,exec" {
		t.Fatal(calls)
	}
	data, _ := os.ReadFile(install + ".old")
	if string(data) != "old" {
		t.Fatal("backup not retained")
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("candidate not retained")
	}
}
func TestWindowsRollbackOrdering(t *testing.T) {
	calls := 0
	expected := errors.New("install failed")
	err := replaceWindows("dest", "src", windowsReplaceOps{rename: func(a, b string) error {
		calls++
		if calls == 2 {
			return expected
		}
		return nil
	}, remove: func(string) error { t.Fatal("cleanup before install"); return nil }})
	if !errors.Is(err, expected) || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestConcurrentProductionFetchComplete(t *testing.T) {
	cache := t.TempDir()
	u := New("r", "b", Options{CacheDir: func() (string, error) { return cache, nil }})
	fakeRelease(t, u, func(string) (int, []byte, error) { return 0, nil, nil })
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- u.FetchAndStage(context.Background(), "v1.0.0") }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil && !strings.Contains(err.Error(), "file changed while opening") {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(cache, "b", "staged", "v2.0.0", u.binaryName()))
	if err != nil || string(data) != "release executable" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestMultiProcessPublication(t *testing.T) {
	if dest := os.Getenv("BACKSCROLL_TEST_PUBLICATION"); dest != "" {
		payload := strings.Repeat("complete", 10000)
		if err := writeAtomic(context.Background(), dest, strings.NewReader(payload), 0755); err != nil {
			t.Fatal(err)
		}
		return
	}
	dest := filepath.Join(t.TempDir(), "binary")
	var cmds []*exec.Cmd
	for i := 0; i < 4; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestMultiProcessPublication$")
		cmd.Env = append(os.Environ(), "BACKSCROLL_TEST_PUBLICATION="+dest)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(data, []byte(strings.Repeat("complete", 10000))) {
		t.Fatalf("incomplete publication size=%d err=%v", len(data), err)
	}
}
func TestCanceledCopyPublishesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &cancelCopyReader{cancel: cancel}
	dest := filepath.Join(t.TempDir(), "binary")
	if err := writeAtomic(ctx, dest, r, 0755); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("canceled payload published")
	}
}

type cancelCopyReader struct{ cancel context.CancelFunc }

func (r *cancelCopyReader) Read(p []byte) (int, error) {
	copy(p, "partial")
	r.cancel()
	return 7, io.EOF
}

func TestLatestLimitAndCanceledBody(t *testing.T) {
	u := New("r", "b")
	cache := t.TempDir()
	u.cacheDir = func() (string, error) { return cache, nil }
	fakeRelease(t, u, func(path string) (int, []byte, error) {
		if strings.HasSuffix(path, "latest") {
			return 200, bytes.Repeat([]byte(" "), int(maxLatestBytes+1)), nil
		}
		return 0, nil, nil
	})
	if err := u.FetchAndStage(context.Background(), "v1.0.0"); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatal(err)
	}
}
func TestRestrictiveTempAndShortWrite(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "binary")
	ops := publicationOps{write: func(f *os.File, p []byte) (int, error) {
		info, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatal(info.Mode())
		}
		return len(p) - 1, nil
	}}
	if err := writeAtomicWith(context.Background(), dest, strings.NewReader("payload"), 0755, ops); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("short write published")
	}
}
func TestWindowsExistingBackupPreserved(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "binary")
	_ = os.WriteFile(dest+".old", []byte("backup"), 0600)
	err := replaceWindows(dest, "unused", windowsReplaceOps{rename: func(string, string) error { t.Fatal("overwriting retained backup"); return nil }, remove: os.Remove})
	if err == nil {
		t.Fatal("existing backup accepted")
	}
	data, _ := os.ReadFile(dest + ".old")
	if string(data) != "backup" {
		t.Fatal("backup lost")
	}
}

func TestProductionFetchInterleavedPublication(t *testing.T) {
	cache := t.TempDir()
	u := New("r", "b", Options{CacheDir: func() (string, error) { return cache, nil }})
	fakeRelease(t, u, func(string) (int, []byte, error) { return 0, nil, nil })
	aReached, bReached, aResume, bResume := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var attempts atomic.Int32
	u.publish = func(ctx context.Context, dest string, r io.Reader, mode os.FileMode) error {
		first := attempts.Add(1) == 1
		reader := &publicationGate{r: r, before: !first}
		if first {
			reader.reached = aReached
			reader.resume = aResume
		} else {
			reader.reached = bReached
			reader.resume = bResume
		}
		return writeAtomic(ctx, dest, reader, mode)
	}
	aDone, bDone := make(chan error, 1), make(chan error, 1)
	go func() { aDone <- u.FetchAndStage(context.Background(), "v1.0.0") }()
	<-aReached
	go func() { bDone <- u.FetchAndStage(context.Background(), "v1.0.0") }()
	<-bReached
	close(aResume)
	if err := <-aDone; err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(cache, "b", "staged", "v2.0.0", u.binaryName())
	data, _ := os.ReadFile(dest)
	if string(data) != "release executable" {
		t.Fatalf("partial candidate %q", data)
	}
	close(bResume)
	if err := <-bDone; err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(dest)
	if string(data) != "release executable" {
		t.Fatalf("mutated candidate %q", data)
	}
}

type publicationGate struct {
	r               io.Reader
	before, started bool
	reached, resume chan struct{}
}

func (g *publicationGate) Read(p []byte) (int, error) {
	if !g.started {
		g.started = true
		if g.before {
			close(g.reached)
			<-g.resume
		}
	}
	n, err := g.r.Read(p)
	if err == io.EOF && !g.before {
		close(g.reached)
		<-g.resume
	}
	return n, err
}

type cancelInFrame struct {
	context.Context
	cancel context.CancelFunc
	frame  string
	hit    bool
}

func (c *cancelInFrame) Err() error {
	pcs := make([]uintptr, 30)
	n := runtime.Callers(1, pcs)
	for _, pc := range pcs[:n] {
		if f := runtime.FuncForPC(pc); f != nil && strings.Contains(f.Name(), c.frame) {
			c.hit = true
			c.cancel()
			break
		}
	}
	return c.Context.Err()
}
func TestCancellationThroughExtractionCopyAndPublication(t *testing.T) {
	for _, phase := range []string{"tar", "zip", "copy"} {
		t.Run(phase, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &cancelInFrame{Context: base, cancel: cancel}
			u := New("r", "b")
			dest := filepath.Join(t.TempDir(), u.binaryName())
			var err error
			switch phase {
			case "tar":
				ctx.frame = ".extractTarBounded"
				var archive bytes.Buffer
				gz := gzip.NewWriter(&archive)
				tr := tar.NewWriter(gz)
				_ = tr.WriteHeader(&tar.Header{Name: u.binaryName(), Size: 7, Mode: 0755})
				_, _ = tr.Write([]byte("payload"))
				_ = tr.Close()
				_ = gz.Close()
				err = u.extractTarContext(ctx, archive.Bytes(), dest)
			case "zip":
				ctx.frame = ".ReadAt"
				var archive bytes.Buffer
				z := zip.NewWriter(&archive)
				w, _ := z.Create(u.binaryName())
				_, _ = w.Write([]byte("payload"))
				_ = z.Close()
				err = u.extractZipContext(ctx, archive.Bytes(), dest)
			case "copy":
				ctx.frame = ".writeAtomicWith"
				u, _, install := stagedFixture(t)
				dest = install
				err = u.ApplyStagedContext(ctx)
				data, _ := os.ReadFile(dest)
				if string(data) != "old" {
					t.Fatal("canceled apply replaced installation")
				}
				dest = ""
			}
			if !ctx.hit || !errors.Is(err, context.Canceled) {
				t.Fatalf("hit=%v error=%v", ctx.hit, err)
			}
			if dest != "" {
				if _, err := os.Stat(dest); !os.IsNotExist(err) {
					t.Fatal("canceled publication survived")
				}
			}
		})
	}
}

func TestArchiveLimitsBeforePublication(t *testing.T) {
	u := New("r", "b")
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tr := tar.NewWriter(gz)
	_ = tr.WriteHeader(&tar.Header{Name: u.binaryName(), Size: 10000, Mode: 0755})
	_, _ = tr.Write(bytes.Repeat([]byte("x"), 10000))
	_ = tr.Close()
	_ = gz.Close()
	for _, limits := range [][2]int64{{1024, 20000}, {20000, 100}} {
		dest := filepath.Join(t.TempDir(), u.binaryName())
		if err := u.extractTarBounded(context.Background(), archive.Bytes(), dest, limits[0], limits[1]); err == nil || !strings.Contains(err.Error(), "limit") {
			t.Fatal(err)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Fatal("limit failure published")
		}
	}
	archive.Reset()
	z := zip.NewWriter(&archive)
	w, _ := z.Create(u.binaryName())
	_, _ = w.Write([]byte("payload"))
	_ = z.Close()
	data := archive.Bytes()
	at := bytes.Index(data, []byte{0x50, 0x4b, 0x01, 0x02})
	if at < 0 {
		t.Fatal("zip central directory missing")
	}
	binary.LittleEndian.PutUint32(data[at+24:], uint32(maxExpandedBytes+1))
	dest := filepath.Join(t.TempDir(), u.binaryName())
	if err := u.extractZipContext(context.Background(), data, dest); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatal(err)
	}
}
