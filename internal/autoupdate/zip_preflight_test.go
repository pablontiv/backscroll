package autoupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// zipBudgetFixture writes real ZIP64 metadata without allocating or expanding
// irrelevant payloads. Their deliberately inconsistent data must never be read.
func zipBudgetFixture(t *testing.T, target string, sizes []uint64, targetAt int) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for i := 0; i <= len(sizes); i++ {
		if i == targetAt {
			w, err := z.Create(target)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("complete")); err != nil {
				t.Fatal(err)
			}
		}
		if i < len(sizes) {
			h := &zip.FileHeader{Name: "irrelevant", Method: zip.Store, UncompressedSize64: sizes[i], CompressedSize64: 1}
			w, err := z.CreateRaw(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestZipArchiveWideBudgetBeforePublication(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sizes    []uint64
		targetAt int
		accept   bool
	}{
		{"target first oversized trailing", []uint64{uint64(maxExpandedBytes) + 1}, 0, false},
		{"target last oversized prefix", []uint64{uint64(maxExpandedBytes) + 1}, 1, false},
		{"cumulative trailing", []uint64{uint64(maxExpandedBytes) / 2, uint64(maxExpandedBytes) / 2}, 0, false},
		{"zip64 maximum trailing", []uint64{^uint64(0)}, 0, false},
		{"zip64 wraparound", []uint64{1, ^uint64(0)}, 0, false},
		{"exact boundary", []uint64{uint64(maxExpandedBytes) - 8}, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				u := New("r", "b")
				dir := t.TempDir()
				dest := filepath.Join(dir, u.binaryName())
				if existing {
					if err := os.WriteFile(dest, []byte("seed"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				err := u.extractZipContext(context.Background(), zipBudgetFixture(t, u.binaryName(), tc.sizes, tc.targetAt), dest)
				if tc.accept {
					if err != nil {
						t.Fatal(err)
					}
					data, _ := os.ReadFile(dest)
					if string(data) != "complete" {
						t.Fatal("incomplete publication")
					}
					continue
				}
				if err == nil || !strings.Contains(err.Error(), "limit") {
					t.Fatalf("budget rejection: %v", err)
				}
				assertZipDestinationUnchanged(t, dir, dest, existing)
			}
		})
	}
}

func assertZipDestinationUnchanged(t *testing.T, dir, dest string, existing bool) {
	t.Helper()
	data, err := os.ReadFile(dest)
	if existing {
		if err != nil || string(data) != "seed" {
			t.Fatalf("destination changed: %q %v", data, err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("destination published: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	if existing {
		want = 1
	}
	if len(entries) != want {
		t.Fatalf("temporary publication survived: %v", entries)
	}
}

type cancelZipMetadata struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *cancelZipMetadata) Err() error {
	pc, _, _, _ := runtime.Caller(1)
	if f := runtime.FuncForPC(pc); f != nil && strings.HasSuffix(f.Name(), ".extractZipContext") {
		c.checks++
		if c.checks == 2 {
			c.cancel()
		}
	}
	return c.Context.Err()
}
func TestZipMetadataCancellationBeforePublication(t *testing.T) {
	for _, existing := range []bool{false, true} {
		u := New("r", "b")
		dir := t.TempDir()
		dest := filepath.Join(dir, u.binaryName())
		if existing {
			if err := os.WriteFile(dest, []byte("seed"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		base, cancel := context.WithCancel(context.Background())
		defer cancel()
		ctx := &cancelZipMetadata{Context: base, cancel: cancel}
		err := u.extractZipContext(ctx, zipBudgetFixture(t, u.binaryName(), []uint64{1, 1}, 0), dest)
		if !errors.Is(err, context.Canceled) || ctx.checks != 2 {
			t.Fatalf("metadata cancellation: checks=%d err=%v", ctx.checks, err)
		}
		assertZipDestinationUnchanged(t, dir, dest, existing)
	}
}

func TestZipMalformedMetadataBeforePublication(t *testing.T) {
	for _, existing := range []bool{false, true} {
		u := New("r", "b")
		dir := t.TempDir()
		dest := filepath.Join(dir, u.binaryName())
		if existing {
			if err := os.WriteFile(dest, []byte("seed"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		data := zipBudgetFixture(t, u.binaryName(), []uint64{^uint64(0)}, 0)
		// Truncate the end-of-central-directory record, leaving unreadable metadata.
		err := u.extractZipContext(context.Background(), data[:len(data)-10], dest)
		if err == nil {
			t.Fatal("malformed metadata accepted")
		}
		assertZipDestinationUnchanged(t, dir, dest, existing)
	}
}
