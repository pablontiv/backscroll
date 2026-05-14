package embedding

import (
	"context"
	"errors"
	"testing"
)

func TestOnnxProvider_ImplementsInterface(t *testing.T) {
	var _ EmbeddingProvider = &OnnxProvider{}
}

func TestOnnxProvider_NewReturnsError(t *testing.T) {
	_, err := NewOnnxProvider("/nonexistent/model.onnx")
	if err == nil {
		t.Fatal("expected error from NewOnnxProvider")
	}
	if !errors.Is(err, ErrOnnxNotAvailable) {
		t.Errorf("expected ErrOnnxNotAvailable, got %v", err)
	}
}

func TestOnnxProvider_Embed_ReturnsError(t *testing.T) {
	p := &OnnxProvider{}
	_, err := p.Embed(context.Background(), "hello")
	if !errors.Is(err, ErrOnnxNotAvailable) {
		t.Errorf("expected ErrOnnxNotAvailable, got %v", err)
	}
}

func TestOnnxProvider_Dimensions(t *testing.T) {
	p := &OnnxProvider{}
	if p.Dimensions() != 384 {
		t.Errorf("Dimensions() = %d, want 384", p.Dimensions())
	}
}

func TestOnnxProvider_Close(t *testing.T) {
	p := &OnnxProvider{}
	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}
