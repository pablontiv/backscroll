package readers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pablontiv/backscroll/internal/input_config"
)

func TestSessionReadersCanceledBeforeWork(t *testing.T) {
	readers := []SessionReader{
		&ClaudeReader{},
		&PiReader{},
		&CodexReader{},
		&OpenCodeReader{},
		&MarkdownDocumentReader{},
		&MarkdownSectionsReader{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	def := input_config.InputDefinition{}
	missing := filepath.Join(t.TempDir(), "missing")

	for _, reader := range readers {
		t.Run(reader.Name(), func(t *testing.T) {
			if _, err := reader.Discover(ctx, def); !errors.Is(err, context.Canceled) {
				t.Fatalf("Discover() error = %v, want context.Canceled", err)
			}
			if _, err := reader.Hash(ctx, missing); !errors.Is(err, context.Canceled) {
				t.Fatalf("Hash() error = %v, want context.Canceled", err)
			}
			if _, err := reader.Parse(ctx, missing, def); !errors.Is(err, context.Canceled) {
				t.Fatalf("Parse() error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestHashFileMatchesSHA256(t *testing.T) {
	content := []byte("backscroll\nwith multiple blocks\n")
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := hashFile(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := sha256.Sum256(content)
	want := hex.EncodeToString(wantBytes[:])
	if got != want {
		t.Fatalf("hashFile() = %q, want %q", got, want)
	}
}

type cancelingHashReader struct {
	cancel context.CancelFunc
	read   bool
}

func (r *cancelingHashReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, nil
	}
	r.read = true
	n := copy(p, bytes.Repeat([]byte("x"), hashBufferSize))
	r.cancel()
	return n, nil
}

func TestHashReaderCanceledDuringRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	_, err := hashReader(ctx, &cancelingHashReader{cancel: cancel})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("hashReader() error = %v, want context.Canceled", err)
	}
}

func TestOpenCodeQueriesHonorCanceledContext(t *testing.T) {
	dbPath := createOpenCodeDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reader := &OpenCodeReader{}
	if _, err := reader.Hash(ctx, dbPath); !errors.Is(err, context.Canceled) {
		t.Fatalf("Hash() error = %v, want context.Canceled", err)
	}
	if _, err := reader.Parse(ctx, dbPath, input_config.InputDefinition{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Parse() error = %v, want context.Canceled", err)
	}
}
