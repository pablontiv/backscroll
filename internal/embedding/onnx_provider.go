package embedding

import (
	"context"
	"errors"
)

// ErrOnnxNotAvailable is returned when the ONNX provider is used but the
// binary was not compiled with ONNX runtime support.
//
// Decision: both github.com/knights-analytics/hugot and
// github.com/yalue/onnxruntime_go require CGO or a native ONNX Runtime
// shared library at build time. The project rule is "no CGO if avoidable"
// (using modernc.org/sqlite for the same reason). Until a pure-Go ONNX
// inference path is available (e.g., github.com/owulveryck/onnx-go with
// full operator support), the ONNX provider is implemented as a stub that
// returns this error. Use MockEmbeddingProvider for all tests.
//
// To enable the real ONNX provider: add a build-tagged file
// onnx_provider_real.go with `//go:build hugot` and wire hugot's pipeline
// behind NewOnnxProvider. CI without the native library continues to use
// the stub.
var ErrOnnxNotAvailable = errors.New("ONNX provider not available: rebuild with -tags hugot and install the ONNX Runtime library")

// OnnxProvider generates embeddings using an ONNX model (all-MiniLM-L6-v2,
// 384 dimensions). Requires the ONNX Runtime native library at runtime.
//
// In this build, all methods return ErrOnnxNotAvailable.
// Use NewMockProvider for testing.
type OnnxProvider struct{}

// NewOnnxProvider loads an ONNX embedding model from modelPath.
// Returns ErrOnnxNotAvailable in the default (no ONNX runtime) build.
func NewOnnxProvider(_ string) (*OnnxProvider, error) {
	return nil, ErrOnnxNotAvailable
}

// Embed returns ErrOnnxNotAvailable.
func (p *OnnxProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, ErrOnnxNotAvailable
}

// Dimensions returns 384 (all-MiniLM-L6-v2 output dimension).
func (p *OnnxProvider) Dimensions() int { return 384 }

// Close is a no-op.
func (p *OnnxProvider) Close() error { return nil }
