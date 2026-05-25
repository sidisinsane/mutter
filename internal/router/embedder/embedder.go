// Package embedder provides semantic embedding generation using ONNX sentence
// transformer models for script matching.
package embedder

import (
	"context"
)

// Embedder generates embedding vectors from text for semantic similarity matching.
type Embedder interface {
	// Embed generates an embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// Dimensions returns the dimensionality of the embedding vectors produced.
	Dimensions() int
	// Close releases any resources held by the embedder.
	Close() error
}

// Config holds configuration for the ONNX embedder.
type Config struct {
	// ModelPath is the absolute path to the ONNX model file.
	ModelPath string
	// ExpectedDimensions is the expected output dimensionality.
	// Validated at startup; mismatch is a hard error.
	ExpectedDimensions int
}

// New creates a new Embedder based on the configuration.
// Currently supports ONNX sentence transformer models.
func New(cfg Config) (Embedder, error) {
	return NewONNXEmbedder(cfg.ModelPath, cfg.ExpectedDimensions)
}
