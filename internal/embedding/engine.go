// Package embedding provides text embedding via ONNX Runtime.
//
// The Engine interface is always available. The ONNX implementation
// requires the "embedding" build tag and native dependencies
// (libonnxruntime.so + libtokenizers.a). See docs/contributing/embedding-setup.md.
package embedding

import "context"

// Engine produces dense vector embeddings from text.
type Engine interface {
	// Embed returns a normalized embedding vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch returns normalized embedding vectors for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dims returns the dimensionality of embedding vectors (e.g. 384).
	Dims() int

	// ModelID returns the model identifier (e.g. "bge-small-en-v1.5").
	// Stored alongside vectors to detect model changes.
	ModelID() string

	// Close releases all resources (ONNX session, tokenizer).
	Close() error
}
