package embedding

import (
	"context"
	"math"
	"testing"
)

func TestMockEngine_Interface(t *testing.T) {
	var _ Engine = (*MockEngine)(nil)
}

func TestMockEngine_Dims(t *testing.T) {
	m := NewMockEngine(384)
	if m.Dims() != 384 {
		t.Fatalf("expected 384, got %d", m.Dims())
	}
}

func TestMockEngine_ModelID(t *testing.T) {
	m := NewMockEngine(384)
	if m.ModelID() != "mock" {
		t.Fatalf("expected mock, got %s", m.ModelID())
	}
}

func TestMockEngine_Embed(t *testing.T) {
	m := NewMockEngine(384)
	ctx := context.Background()

	v, err := m.Embed(ctx, "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(v))
	}

	// Check L2 norm ≈ 1.0.
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 1e-5 {
		t.Fatalf("expected unit norm, got %f", norm)
	}
}

func TestMockEngine_Deterministic(t *testing.T) {
	m := NewMockEngine(384)
	ctx := context.Background()

	v1, _ := m.Embed(ctx, "test input")
	v2, _ := m.Embed(ctx, "test input")
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("not deterministic at index %d", i)
		}
	}
}

func TestMockEngine_DifferentInputs(t *testing.T) {
	m := NewMockEngine(384)
	ctx := context.Background()

	v1, _ := m.Embed(ctx, "hello")
	v2, _ := m.Embed(ctx, "goodbye")

	same := true
	for i := range v1 {
		if v1[i] != v2[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different inputs produced identical vectors")
	}
}

func TestMockEngine_EmbedBatch(t *testing.T) {
	m := NewMockEngine(384)
	ctx := context.Background()

	vecs, err := m.EmbedBatch(ctx, []string{"a", "b", "c"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
	for i, v := range vecs {
		if len(v) != 384 {
			t.Fatalf("vector %d: expected 384 dims, got %d", i, len(v))
		}
	}
}

func TestMockEngine_Close(t *testing.T) {
	m := NewMockEngine(384)
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}
