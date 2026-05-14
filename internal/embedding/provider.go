// Package embedding provides the EmbeddingProvider interface and implementations
// for generating vector embeddings from text.
package embedding

import "context"

// EmbeddingProvider generates fixed-dimension vector embeddings for text.
type EmbeddingProvider interface {
	// Embed generates a vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// Dimensions returns the output vector dimension (e.g., 384).
	Dimensions() int
	// Close releases resources held by the provider.
	Close() error
}
