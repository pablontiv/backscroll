package autoupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type cancelAtPublication struct {
	context.Context
	cancel context.CancelFunc
}

func (c cancelAtPublication) Err() error {
	err := c.Context.Err()
	pc, _, _, _ := runtime.Caller(1)
	if strings.HasSuffix(runtime.FuncForPC(pc).Name(), ".FetchAndStage") {
		c.cancel()
	}
	return err
}
func TestIndependentCancellationDuringExtraction(t *testing.T) {
	cache := t.TempDir()
	u := New("r", "b", Options{CacheDir: func() (string, error) { return cache, nil }})
	fakeRelease(t, u, func(p string) (int, []byte, error) { return 0, nil, nil })
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := cancelAtPublication{base, cancel}
	done := make(chan error, 1)
	go func() { done <- u.FetchAndStage(ctx, "v1.0.0") }()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation error, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fetch timeout")
	}
	dest := filepath.Join(cache, "b", "staged", "v2.0.0", u.binaryName())
	if base.Err() != nil {
		if data, err := os.ReadFile(dest); err == nil {
			t.Fatalf("FetchAndStage returned nil and published %q after cancellation at final pre-extraction check", data)
		}
	}
}

type gateReader struct {
	first   bool
	before  bool
	reached chan struct{}
	resume  chan struct{}
}

func (r *gateReader) Read(p []byte) (int, error) {
	if !r.first {
		r.first = true
		if r.before {
			close(r.reached)
			<-r.resume
		}
		copy(p, "abc")
		return 3, nil
	}
	if !r.before {
		close(r.reached)
		<-r.resume
	}
	return 0, io.EOF
}
func TestIndependentConcurrentPublication(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "binary")
	a := &gateReader{reached: make(chan struct{}), resume: make(chan struct{})}
	b := &gateReader{before: true, reached: make(chan struct{}), resume: make(chan struct{})}
	adone, bdone := make(chan error, 1), make(chan error, 1)
	go func() { adone <- writeAtomic(context.Background(), dest, a, 0755) }()
	<-a.reached
	go func() { bdone <- writeAtomic(context.Background(), dest, b, 0755) }()
	<-b.reached
	close(a.resume)
	if err := <-adone; err != nil {
		t.Fatal(err)
	}
	partial, _ := os.ReadFile(dest)
	close(b.resume)
	second := <-bdone
	got, _ := os.ReadFile(dest)
	if len(partial) != 3 || second != nil {
		t.Fatalf("publisher A succeeded with partial executable=%q; writer B later mutated it to %q and returned %v", partial, got, second)
	}
}
