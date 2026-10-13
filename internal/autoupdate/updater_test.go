package autoupdate

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
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFetchTransportFailure(t *testing.T) {
	expected := errors.New("network failure")
	u := New("pablontiv/backscroll", "backscroll")
	u.httpClient = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return nil, expected })}
	if err := u.FetchAndStage(context.Background(), "v1.0.0"); !errors.Is(err, expected) {
		t.Fatalf("error=%v", err)
	}
}

func releaseArchive(t *testing.T, name string, payload []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if runtime.GOOS == "windows" {
		w := zip.NewWriter(&out)
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&out)
		w := tar.NewWriter(gz)
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(payload))}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return out.Bytes()
}
func fakeRelease(t *testing.T, u *Updater, mutate func(string) (int, []byte, error)) {
	t.Helper()
	archive := releaseArchive(t, u.binaryName(), []byte("release executable"))
	sum := sha256.Sum256(archive)
	u.httpClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		code, body, err := mutate(r.URL.Path)
		if err != nil {
			return nil, err
		}
		if body == nil && code == 0 {
			code = 200
			switch {
			case strings.HasSuffix(r.URL.Path, "/latest"):
				body = []byte(`{"tag_name":"v2.0.0"}`)
			case strings.HasSuffix(r.URL.Path, "checksums.txt"):
				body = []byte(hex.EncodeToString(sum[:]) + "  " + u.archiveName("v2.0.0") + "\n")
			default:
				body = archive
			}
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
}

func TestFetchErrorClasses(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		code       int
		body       []byte
		transport  bool
	}{
		{"api status", "/latest", 503, []byte("unavailable"), false},
		{"api json", "/latest", 200, []byte("{"), false},
		{"api tag", "/latest", 200, []byte("{}"), false},
		{"archive status", ".tar.gz", 404, []byte("missing"), false},
		{"zip archive status", ".zip", 404, []byte("missing"), false},
		{"checksum status", "checksums.txt", 403, []byte("denied"), false},
		{"checksum missing", "checksums.txt", 200, []byte("other"), false},
		{"checksum mismatch", "checksums.txt", 200, []byte("bad  ARCHIVE"), false},
		{"archive network", "ARCHIVE", 0, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := t.TempDir()
			u := New("pablontiv/backscroll", "backscroll", Options{CacheDir: func() (string, error) { return cache, nil }})
			path := strings.ReplaceAll(tc.path, "ARCHIVE", u.archiveName("v2.0.0"))
			body := bytes.ReplaceAll(tc.body, []byte("ARCHIVE"), []byte(u.archiveName("v2.0.0")))
			if (tc.path == ".zip" && runtime.GOOS != "windows") || (tc.path == ".tar.gz" && runtime.GOOS == "windows") {
				t.Skip("other platform archive")
			}
			fakeRelease(t, u, func(p string) (int, []byte, error) {
				if strings.HasSuffix(p, path) {
					if tc.transport {
						return 0, nil, errors.New("transport")
					}
					return tc.code, body, nil
				}
				return 0, nil, nil
			})
			if err := u.FetchAndStage(context.Background(), "v1.0.0"); err == nil {
				t.Fatal("failure swallowed")
			}
			dest := filepath.Join(cache, u.Binary, "staged", "v2.0.0", u.binaryName())
			if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial staged executable: %v", err)
			}
		})
	}
}
func TestFetchSuccessAndParity(t *testing.T) {
	cache := t.TempDir()
	u := New("pablontiv/backscroll", "backscroll", Options{CacheDir: func() (string, error) { return cache, nil }})
	fakeRelease(t, u, func(string) (int, []byte, error) { return 0, nil, nil })
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	want := fmt.Sprintf("backscroll_2.0.0_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
	if u.archiveName("v2.0.0") != want {
		t.Fatal(u.archiveName("v2.0.0"))
	}
	if err := u.FetchAndStage(context.Background(), "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(cache, "backscroll", "staged", "v2.0.0", u.binaryName())
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "release executable" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	u.httpClient.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"tag_name":"v2.0.0"}`))}, nil
	})
	if err = u.FetchAndStage(context.Background(), "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err = u.FetchAndStage(context.Background(), "v2.0.0"); err != nil {
		t.Fatal(err)
	}
}
func TestFetchCacheAndStagingFailures(t *testing.T) {
	expected := errors.New("cache unavailable")
	u := New("r", "b", Options{CacheDir: func() (string, error) { return "", expected }})
	fakeRelease(t, u, func(string) (int, []byte, error) { return 0, nil, nil })
	if err := u.FetchAndStage(context.Background(), "v1.0.0"); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(cache, "b"), []byte("not directory"), 0600); err != nil {
		t.Fatal(err)
	}
	u.cacheDir = func() (string, error) { return cache, nil }
	if err := u.FetchAndStage(context.Background(), "v1.0.0"); err == nil {
		t.Fatal("staging failure swallowed")
	}
}
func TestFetchCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer server.Close()
	u := New("r", "b")
	u.githubAPI = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- u.FetchAndStage(ctx, "v1.0.0") }()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("request not canceled")
	}
}
func TestDevExemption(t *testing.T) {
	u := New("r", "b", Options{CacheDir: func() (string, error) { t.Fatal("dev cache access"); return "", nil }})
	u.httpClient.Transport = transportFunc(func(*http.Request) (*http.Response, error) { t.Fatal("dev HTTP"); return nil, nil })
	u.CurrentVersion = "dev"
	if err := u.FetchAndStage(context.Background(), "dev"); err != nil {
		t.Fatal(err)
	}
	if err := u.ApplyStagedIfAvailable(); err != nil {
		t.Fatal(err)
	}
}
func TestExtractionFailuresNoPartialBinary(t *testing.T) {
	u := New("r", "b")
	dest := filepath.Join(t.TempDir(), u.binaryName())
	if err := u.extractTar([]byte("broken"), dest); err == nil {
		t.Fatal("gzip error swallowed")
	}
	if err := u.extractZip([]byte("broken"), dest); err == nil {
		t.Fatal("zip error swallowed")
	}
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tr := tar.NewWriter(gz)
	_ = tr.WriteHeader(&tar.Header{Name: u.binaryName(), Mode: 0755, Size: 100})
	_, _ = tr.Write([]byte("short"))
	_ = gz.Close()
	if err := u.extractTar(out.Bytes(), dest); err == nil {
		t.Fatal("truncated binary accepted")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial binary")
	}
	missing := releaseArchive(t, "not-the-binary", []byte("x"))
	if runtime.GOOS == "windows" {
		if err := u.extractZip(missing, dest); err == nil {
			t.Fatal("missing zip member")
		}
	} else {
		if err := u.extractTar(missing, dest); err == nil {
			t.Fatal("missing tar member")
		}
	}
}

func TestVersionParity(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{{"v2.0.0", "1.0.0", true}, {"1.2.4-alpha+build", "v1.2.3", true}, {"v1.2.3", "1.2.3", false}, {"bad", "1.0.0", false}, {"1.-1.0", "1.0.0", false}, {"1.2", "1.0.0", false}, {"1.1.9", "1.2.0", false}} {
		if got := isNewer(tc.a, tc.b); got != tc.want {
			t.Fatalf("%s > %s = %v", tc.a, tc.b, got)
		}
	}
}

type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("body read failure") }
func (brokenBody) Close() error             { return nil }
func TestDownloadAndStagingWriteFailures(t *testing.T) {
	u := New("r", "b")
	if u.httpClient.Timeout != 60*time.Second {
		t.Fatal(u.httpClient.Timeout)
	}
	if _, err := u.download(context.Background(), ":bad"); err == nil {
		t.Fatal("invalid URL accepted")
	}
	u.httpClient.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: brokenBody{}}, nil
	})
	if _, err := u.download(context.Background(), "https://example.invalid"); err == nil {
		t.Fatal("read failure swallowed")
	}
	cache := t.TempDir()
	u.cacheDir = func() (string, error) { return cache, nil }
	fakeRelease(t, u, func(path string) (int, []byte, error) {
		if strings.HasSuffix(path, "checksums.txt") {
			if err := os.WriteFile(filepath.Join(cache, "b"), []byte("block mkdir"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return 0, nil, nil
	})
	if err := u.FetchAndStage(context.Background(), "v1.0.0"); err == nil || !strings.Contains(err.Error(), "staging directory") {
		t.Fatalf("err=%v", err)
	}
}

func TestZipExtractionAndWriteErrors(t *testing.T) {
	u := New("r", "b")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create("dir/" + u.binaryName())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("zip binary"))
	_ = z.Close()
	dest := filepath.Join(t.TempDir(), u.binaryName())
	if err := u.extractZip(buf.Bytes(), dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "zip binary" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	blocked := filepath.Join(t.TempDir(), "directory")
	_ = os.Mkdir(blocked, 0755)
	if err := u.extractZip(buf.Bytes(), blocked); err == nil {
		t.Fatal("rename error swallowed")
	}
	if _, err := os.Stat(blocked + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("temporary file retained")
	}
	buf.Reset()
	z = zip.NewWriter(&buf)
	_, _ = z.Create("wrong")
	_ = z.Close()
	if err := u.extractZip(buf.Bytes(), dest); err == nil {
		t.Fatal("missing zip binary")
	}
}

func TestGzipIntegrityBeforePublish(t *testing.T) {
	u := New("r", "b")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tr := tar.NewWriter(gz)
	_ = tr.WriteHeader(&tar.Header{Name: u.binaryName(), Size: 4, Mode: 0755})
	_, _ = tr.Write([]byte("good"))
	_ = tr.Close()
	_ = gz.Close()
	data := buf.Bytes()
	data[len(data)-8] ^= 1
	dest := filepath.Join(t.TempDir(), u.binaryName())
	if err := u.extractTar(data, dest); err == nil {
		t.Fatal("bad gzip CRC accepted")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("published corrupt archive: %v", err)
	}
}

type closeFailureBody struct{ io.Reader }

func (closeFailureBody) Close() error { return errors.New("download close failure") }
func TestDownloadCloseFailure(t *testing.T) {
	u := New("r", "b")
	u.httpClient.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: closeFailureBody{strings.NewReader("body")}}, nil
	})
	if _, err := u.download(context.Background(), "https://example.invalid"); err == nil || !strings.Contains(err.Error(), "download close failure") {
		t.Fatal(err)
	}
}
func TestZipChecksumFailureNoPartialBinary(t *testing.T) {
	u := New("r", "b")
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.CreateHeader(&zip.FileHeader{Name: u.binaryName(), Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("good"))
	_ = z.Close()
	data := buf.Bytes()
	at := bytes.Index(data, []byte("good"))
	if at < 0 {
		t.Fatal("fixture payload missing")
	}
	data[at] ^= 1
	dest := filepath.Join(t.TempDir(), u.binaryName())
	if err := u.extractZip(data, dest); err == nil {
		t.Fatal("zip CRC error swallowed")
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial binary")
	}
	if _, err := os.Stat(dest + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial temporary retained")
	}
}

func TestNoRuntimeOptOut(t *testing.T) {
	t.Setenv("BACKSCROLL_AUTOUPDATE_DISABLE", "1")
	u, _, install := stagedFixture(t)
	calls := 0
	u.exec = func(string) error { calls++; return nil }
	if err := u.ApplyStagedIfAvailable(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(install)
	if calls != 1 || string(data) != "new" {
		t.Fatal("apply disabled by environment")
	}
	fakeRelease(t, u, func(string) (int, []byte, error) { calls++; return 0, nil, nil })
	if err := u.FetchAndStage(context.Background(), "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("fetch disabled by environment: calls=%d", calls)
	}
}
