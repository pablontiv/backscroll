package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

// MockEmbeddingProvider returns deterministic vectors for testing without
// requiring a real ONNX model. Different texts produce different vectors.
type MockEmbeddingProvider struct {
	dims int
}

// NewMockProvider creates a MockEmbeddingProvider with the given vector dimension.
func NewMockProvider(dims int) *MockEmbeddingProvider {
	return &MockEmbeddingProvider{dims: dims}
}

// Embed returns a deterministic vector derived from the SHA-256 hash of the text.
// The first min(8, dims) values are derived from the hash; the rest are zero.
func (m *MockEmbeddingProvider) Embed(_ context.Context, text string) ([]float32, error) {
	vec := make([]float32, m.dims)
	if m.dims == 0 {
		return vec, nil
	}

	h := sha256.Sum256([]byte(text))
	// Fill as many float32 values as we have hash bytes for (up to dims)
	for i := 0; i+4 <= len(h) && i/4 < m.dims; i += 4 {
		bits := binary.LittleEndian.Uint32(h[i : i+4])
		// Normalize to [-1, 1]
		vec[i/4] = (float32(bits)/float32(^uint32(0)))*2 - 1
	}
	return vec, nil
}

// Dimensions returns the configured vector dimension.
func (m *MockEmbeddingProvider) Dimensions() int { return m.dims }

// Close is a no-op for the mock provider.
func (m *MockEmbeddingProvider) Close() error { return nil }
