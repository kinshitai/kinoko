//go:build embedding

package embedding

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func modelDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	dir := filepath.Join(home, ".kinoko", "models", "bge-small-en-v1.5")
	if _, err := os.Stat(filepath.Join(dir, "model.onnx")); err != nil {
		t.Skipf("model not present at %s — skipping ONNX tests", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "tokenizer.json")); err != nil {
		t.Skipf("tokenizer not present at %s — skipping ONNX tests", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "libonnxruntime.so")); err != nil {
		t.Skipf("libonnxruntime.so not present at %s — skipping ONNX tests", dir)
	}
	return dir
}

func TestONNXEngine_Interface(t *testing.T) {
	var _ Engine = (*ONNXEngine)(nil)
}

func TestONNXEngine_Embed(t *testing.T) {
	dir := modelDir(t)
	engine, err := NewONNXEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctx := context.Background()
	vec, err := engine.Embed(ctx, "Hello world")
	if err != nil {
		t.Fatal(err)
	}

	if len(vec) != 384 {
		t.Fatalf("expected 384 dims, got %d", len(vec))
	}

	// Check L2 norm ≈ 1.0.
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if math.Abs(norm-1.0) > 1e-4 {
		t.Fatalf("expected unit norm, got %f", norm)
	}
}

func TestONNXEngine_EmbedBatch(t *testing.T) {
	dir := modelDir(t)
	engine, err := NewONNXEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctx := context.Background()
	texts := []string{"Go is great", "Rust is fast", "Python is easy"}
	vecs, err := engine.EmbedBatch(ctx, texts)
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

func TestONNXEngine_SimilarTexts(t *testing.T) {
	dir := modelDir(t)
	engine, err := NewONNXEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctx := context.Background()
	v1, _ := engine.Embed(ctx, "The cat sat on the mat")
	v2, _ := engine.Embed(ctx, "A cat was sitting on a mat")
	v3, _ := engine.Embed(ctx, "Stock prices rose sharply today")

	sim12 := cosine(v1, v2)
	sim13 := cosine(v1, v3)

	// Similar sentences should have higher cosine similarity.
	if sim12 <= sim13 {
		t.Fatalf("expected sim(cat1,cat2)=%f > sim(cat,stocks)=%f", sim12, sim13)
	}
}

func TestONNXEngine_Dims(t *testing.T) {
	dir := modelDir(t)
	engine, err := NewONNXEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	if engine.Dims() != 384 {
		t.Fatalf("expected 384, got %d", engine.Dims())
	}
}

func TestONNXEngine_ModelID(t *testing.T) {
	dir := modelDir(t)
	engine, err := NewONNXEngine(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	if engine.ModelID() != "bge-small-en-v1.5" {
		t.Fatalf("expected bge-small-en-v1.5, got %s", engine.ModelID())
	}
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
