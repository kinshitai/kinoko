package embedding

import (
	"context"
	"math"
)

// MockEngine implements Engine with deterministic vectors for testing.
// It produces a simple hash-based vector so different inputs yield
// different (but reproducible) embeddings.
type MockEngine struct {
	dims    int
	modelID string
}

// NewMockEngine creates a MockEngine with the given dimensionality.
func NewMockEngine(dims int) *MockEngine {
	return &MockEngine{dims: dims, modelID: "mock"}
}

func (m *MockEngine) Embed(_ context.Context, text string) ([]float32, error) {
	return m.deterministicVector(text), nil
}

func (m *MockEngine) EmbedBatch(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = m.deterministicVector(t)
	}
	return out, nil
}

func (m *MockEngine) Dims() int       { return m.dims }
func (m *MockEngine) ModelID() string { return m.modelID }
func (m *MockEngine) Close() error    { return nil }

// deterministicVector produces a reproducible L2-normalized vector from text.
func (m *MockEngine) deterministicVector(text string) []float32 {
	v := make([]float32, m.dims)
	// Simple hash spread across dimensions.
	h := uint64(5381)
	for _, c := range text {
		h = h*33 + uint64(c)
	}
	for i := range v {
		h ^= h << 13
		h ^= h >> 7
		h ^= h << 17
		v[i] = float32(int32(h&0xFFFF)-0x8000) / 0x8000 //nolint:gosec // deterministic mock, overflow is intentional
	}
	// L2 normalize.
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range v {
			v[i] = float32(float64(v[i]) / norm)
		}
	}
	return v
}
