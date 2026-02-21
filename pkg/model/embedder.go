package model

import "context"

// Embedder computes vector embeddings for text.
// This interface mirrors embedding.Embedder so that packages outside the
// server-side embedding implementation can depend on a lightweight contract.
// Go structural typing ensures embedding.Client satisfies this automatically.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int
}
