package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pablontiv/backscroll/internal/autoupdate"
	"github.com/spf13/cobra"
)

type backendTransport func(*http.Request) (*http.Response, error)

func (f backendTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProductionUpdaterAdapterFailures(t *testing.T) {
	old := version
	version = "v1.0.0"
	defer func() { version = old }()
	for _, phase := range []string{"fetch", "apply"} {
		t.Run(phase, func(t *testing.T) {
			var out, errOut bytes.Buffer
			var requests atomic.Int32
			client := &http.Client{Timeout: 60 * time.Second, Transport: backendTransport(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				if phase == "fetch" {
					return nil, errors.New("backend network failure")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"v1.0.0"}`))}, nil
			})}
			cache := t.TempDir()
			u := autoupdate.New("pablontiv/backscroll", "backscroll", autoupdate.Options{HTTPClient: client, CacheDir: func() (string, error) { return cache, nil }, Executable: func() (string, error) { return "", errors.New("backend executable failure") }})
			u.CurrentVersion = version
			if phase == "apply" {
				name := "backscroll"
				if runtime.GOOS == "windows" {
					name += ".exe"
				}
				dir := filepath.Join(cache, "backscroll", "staged", "v2.0.0")
				_ = os.MkdirAll(dir, 0755)
				_ = os.WriteFile(filepath.Join(dir, name), []byte("candidate"), 0755)
			}
			root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { out.WriteString("command succeeded\n") }}
			err := runContextWithDependencies(context.Background(), &out, &errOut, nil, updaterRunDependencies(u, root))
			if err != nil || out.String() != "command succeeded\n" || requests.Load() != 1 || strings.Count(errOut.String(), "warning: autoupdate:") != 1 || !strings.Contains(errOut.String(), "backend") {
				t.Fatalf("err=%v stdout=%q stderr=%q calls=%d", err, out.String(), errOut.String(), requests.Load())
			}
		})
	}
}
func TestProductionUpdaterInvalidMatrixUntouched(t *testing.T) {
	old := version
	version = "v1.0.0"
	defer func() { version = old }()
	for _, args := range [][]string{{"--help"}, {"--version"}, {"--skill"}, {"unknown"}, {"purge", "--no-sync"}, {"annotate"}, {"search", "--limit=-1"}, {"context"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			var calls atomic.Int32
			cache := t.TempDir()
			name := "backscroll"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			candidate := filepath.Join(cache, "backscroll", "staged", "v2.0.0", name)
			_ = os.MkdirAll(filepath.Dir(candidate), 0755)
			_ = os.WriteFile(candidate, []byte("untouched"), 0755)
			u := autoupdate.New("pablontiv/backscroll", "backscroll", autoupdate.Options{
				HTTPClient: &http.Client{Transport: backendTransport(func(*http.Request) (*http.Response, error) { calls.Add(1); return nil, errors.New("forbidden HTTP") })},
				CacheDir:   func() (string, error) { calls.Add(1); return cache, nil },
				Executable: func() (string, error) { calls.Add(1); return "", errors.New("forbidden executable") },
			})
			u.CurrentVersion = version
			_ = runContextWithDependencies(context.Background(), &out, &errOut, args, updaterRunDependencies(u, buildRootCmd(&out, &errOut)))
			data, err := os.ReadFile(candidate)
			if calls.Load() != 0 || err != nil || string(data) != "untouched" {
				t.Fatalf("calls=%d data=%q err=%v", calls.Load(), data, err)
			}
		})
	}
}
func TestProductionUpdaterStageAndApply(t *testing.T) {
	old := version
	version = "v1.0.0"
	defer func() { version = old }()
	name := "backscroll"
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		name += ".exe"
		ext = ".zip"
	}
	var archive bytes.Buffer
	if runtime.GOOS == "windows" {
		z := zip.NewWriter(&archive)
		f, _ := z.Create(name)
		_, _ = f.Write([]byte("new"))
		_ = z.Close()
	} else {
		gz := gzip.NewWriter(&archive)
		tr := tar.NewWriter(gz)
		_ = tr.WriteHeader(&tar.Header{Name: name, Size: 3, Mode: 0755})
		_, _ = tr.Write([]byte("new"))
		_ = tr.Close()
		_ = gz.Close()
	}
	sum := sha256.Sum256(archive.Bytes())
	archiveName := fmt.Sprintf("backscroll_2.0.0_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
	cache := t.TempDir()
	install := filepath.Join(t.TempDir(), name)
	_ = os.WriteFile(install, []byte("old"), 0755)
	var execs atomic.Int32
	client := &http.Client{Transport: backendTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch {
		case strings.HasSuffix(r.URL.Path, "latest"):
			body = []byte(`{"tag_name":"v2.0.0"}`)
		case strings.HasSuffix(r.URL.Path, "checksums.txt"):
			body = []byte(hex.EncodeToString(sum[:]) + "  " + archiveName)
		default:
			if !strings.HasSuffix(r.URL.Path, archiveName) {
				t.Errorf("wrong archive request %s", r.URL)
			}
			body = archive.Bytes()
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
	u := autoupdate.New("pablontiv/backscroll", "backscroll", autoupdate.Options{HTTPClient: client, CacheDir: func() (string, error) { return cache, nil }, Executable: func() (string, error) { return install, nil }, Exec: func(string) error { execs.Add(1); return nil }})
	u.CurrentVersion = version
	for i := 0; i < 2; i++ {
		var out, errOut bytes.Buffer
		root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { out.WriteString("ok") }}
		if err := runContextWithDependencies(context.Background(), &out, &errOut, nil, updaterRunDependencies(u, root)); err != nil || out.String() != "ok" || errOut.Len() != 0 {
			t.Fatalf("err=%v stdout=%q stderr=%q", err, out.String(), errOut.String())
		}
	}
	data, _ := os.ReadFile(install)
	if string(data) != "new" || execs.Load() != 1 {
		t.Fatalf("data=%q execs=%d", data, execs.Load())
	}
}

// This reproduces the rejected review's concrete-boundary probe through the
// actual production constructor, not a replacement updater or error closure.
func TestProductionConstructorNetworkFailureWarns(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	oldVersion, oldTransport := version, http.DefaultTransport
	version = "v1.0.0"
	var requests atomic.Int32
	http.DefaultTransport = backendTransport(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return nil, errors.New("production transport failure")
	})
	defer func() { version = oldVersion; http.DefaultTransport = oldTransport }()
	var out, errOut bytes.Buffer
	deps := newRunDependencies(&out, &errOut)
	deps.root = &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { out.WriteString("ok") }}
	if err := runContextWithDependencies(context.Background(), &out, &errOut, nil, deps); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || out.String() != "ok" || strings.Count(errOut.String(), "warning: autoupdate:") != 1 || !strings.Contains(errOut.String(), "production transport failure") {
		t.Fatalf("calls=%d out=%q stderr=%q", requests.Load(), out.String(), errOut.String())
	}
}

func TestProductionUpdaterMachineOutputUnchanged(t *testing.T) {
	_, cleanup := testEnv(t)
	defer cleanup()
	oldVersion, oldTransport := version, http.DefaultTransport
	defer func() { version = oldVersion; http.DefaultTransport = oldTransport }()
	http.DefaultTransport = backendTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("machine-output network failure") })
	for _, flag := range []string{"--json", "--robot"} {
		version = "dev"
		var baseline, baselineErr bytes.Buffer
		if err := runContext(context.Background(), &baseline, &baselineErr, []string{"list", "--all-projects", flag}); err != nil {
			t.Fatal(err)
		}
		version = "v1.0.0"
		var out, errOut bytes.Buffer
		if err := runContext(context.Background(), &out, &errOut, []string{"list", "--all-projects", flag}); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Bytes(), baseline.Bytes()) || strings.Count(errOut.String(), "warning: autoupdate:") != 1 {
			t.Fatalf("flag=%s baseline=%q out=%q stderr=%q", flag, baseline.String(), out.String(), errOut.String())
		}
	}
}

func TestProductionUpdaterCapCancelsRequest(t *testing.T) {
	old := version
	version = "v1.0.0"
	defer func() { version = old }()
	started, exited := make(chan struct{}), make(chan struct{})
	tick := make(chan time.Time, 1)
	u := autoupdate.New("pablontiv/backscroll", "backscroll", autoupdate.Options{
		CacheDir: func() (string, error) { return t.TempDir(), nil },
		HTTPClient: &http.Client{Transport: backendTransport(func(r *http.Request) (*http.Response, error) {
			close(started)
			<-r.Context().Done()
			close(exited)
			return nil, r.Context().Err()
		})},
	})
	u.CurrentVersion = version
	var out, errOut bytes.Buffer
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { <-started; tick <- time.Time{} }}
	deps := updaterRunDependencies(u, root)
	deps.stagingTimer = func(d time.Duration) (<-chan time.Time, func()) {
		if d != 10*time.Second {
			t.Fatalf("wait=%v", d)
		}
		return tick, func() {}
	}
	if err := runContextWithDependencies(context.Background(), &out, &errOut, nil, deps); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("capped request did not exit")
	}
	if strings.Count(errOut.String(), "warning: autoupdate:") != 1 || !strings.Contains(errOut.String(), "staging did not complete within 10s") {
		t.Fatalf("stderr=%q", errOut.String())
	}
}
