package storage

import (
	"math"
	"testing"
	"time"
)

// Tests for store methods that lack coverage — R6 area.

func TestFloat32sRoundTrip(t *testing.T) {
	tests := [][]float32{
		{0.1, 0.2, 0.3},
		{-1.0, 0, 1.0},
		{math.MaxFloat32, math.SmallestNonzeroFloat32},
		{},
		nil,
	}
	for _, input := range tests {
		b := float32sToBytes(input)
		got := bytesToFloat32s(b)
		if len(got) != len(input) {
			t.Errorf("len mismatch: %d vs %d", len(got), len(input))
			continue
		}
		for i := range input {
			if got[i] != input[i] {
				t.Errorf("index %d: %f != %f", i, got[i], input[i])
			}
		}
	}
}

func TestNullString(t *testing.T) {
	ns := nullString("")
	if ns.Valid {
		t.Error("empty string should be invalid")
	}
	ns = nullString("hello")
	if !ns.Valid || ns.String != "hello" {
		t.Errorf("got %v", ns)
	}
}

func TestNullTime(t *testing.T) {
	nt := nullTime(time.Time{})
	if nt.Valid {
		t.Error("zero time should be invalid")
	}
	now := time.Now()
	nt = nullTime(now)
	if !nt.Valid || nt.Time != now {
		t.Errorf("got %v", nt)
	}
}

func TestCosineSimilarity_EdgeCases(t *testing.T) {
	if v := cosineSimilarity([]float32{1, 0}, []float32{1}); v != 0 {
		t.Errorf("different lengths = %f, want 0", v)
	}
	if v := cosineSimilarity([]float32{0, 0, 0}, []float32{0, 0, 0}); v != 0 {
		t.Errorf("zero vectors = %f, want 0", v)
	}
	if v := cosineSimilarity([]float32{1, 0, 0}, []float32{0, 0, 0}); v != 0 {
		t.Errorf("one zero = %f, want 0", v)
	}
	if v := cosineSimilarity([]float32{1, 0, 0}, []float32{-1, 0, 0}); math.Abs(v+1.0) > 0.001 {
		t.Errorf("opposite = %f, want -1", v)
	}
}
