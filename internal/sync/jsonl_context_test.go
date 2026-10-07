package sync

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestIterateJSONLCanceledBeforeOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := IterateJSONLFileContext(ctx, "missing.jsonl", func(int, []byte) error {
		t.Fatal("callback called after cancellation")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("IterateJSONLFileContext() error = %v, want context.Canceled", err)
	}
}

func TestIterateJSONLCanceledDuringLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := iterateJSONL(ctx, strings.NewReader("first\nsecond\n"), "controlled", func(lineNumber int, line []byte) error {
		calls++
		if lineNumber != 1 || string(line) != "first" {
			t.Fatalf("first callback = (%d, %q)", lineNumber, line)
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("iterateJSONL() error = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Fatalf("callback calls = %d, want 1", calls)
	}
}
