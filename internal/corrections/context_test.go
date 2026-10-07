package corrections

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/pablontiv/backscroll/internal/models"
)

type cancelAfterChecksContext struct {
	context.Context
	mu       sync.Mutex
	calls    int
	cancelAt int
	done     chan struct{}
	once     sync.Once
}

func newCancelAfterChecksContext(cancelAt int) *cancelAfterChecksContext {
	return &cancelAfterChecksContext{
		Context:  context.Background(),
		cancelAt: cancelAt,
		done:     make(chan struct{}),
	}
}

func (c *cancelAfterChecksContext) Done() <-chan struct{} {
	return c.done
}

func (c *cancelAfterChecksContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.calls < c.cancelAt {
		return nil
	}
	c.once.Do(func() { close(c.done) })
	return context.Canceled
}

func TestRunDetectorsFilteredContextCancelsMidDetectorLoop(t *testing.T) {
	// Checks 1 and 2 enter the function/message loop, check 3 precedes the
	// lexicon detector, and check 4 observes cancellation after that
	// noncancelable detector boundary. Any accumulated result must be discarded.
	ctx := newCancelAfterChecksContext(4)
	got, err := RunDetectorsFilteredContext(ctx, []models.Message{{
		Role:        "user",
		ContentType: "text",
		Content:     "this is wrong",
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunDetectorsFilteredContext error = %v, want context.Canceled", err)
	}
	if got != nil {
		t.Fatalf("canceled detector result = %#v, want nil", got)
	}
}

func TestRunDetectorsFilteredWrapperCompatibility(t *testing.T) {
	msgs := []models.Message{
		{Role: "assistant", ContentType: "text", Content: "permission denied"},
		{Role: "user", ContentType: "text", Content: "that's wrong"},
	}
	want := RunDetectorsFiltered(msgs)
	got, err := RunDetectorsFilteredContext(context.Background(), msgs)
	if err != nil {
		t.Fatalf("RunDetectorsFilteredContext: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("context detections = %#v, wrapper detections = %#v", got, want)
	}
}
