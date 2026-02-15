package metrics

import (
	"math"
	"testing"
)

func TestTwoProportionZTest(t *testing.T) {
	// Known example: treatment 80/200 (40%), control 60/200 (30%).
	z, p := TwoProportionZTest(80, 200, 60, 200)

	// Expected z ≈ 2.132
	if math.Abs(z-2.132) > 0.05 {
		t.Errorf("z-score: got %.3f, want ~2.132", z)
	}
	// Expected p ≈ 0.033 (significant)
	if p >= 0.05 {
		t.Errorf("expected p < 0.05, got %.4f", p)
	}
}

func TestTwoProportionZTestEqual(t *testing.T) {
	// Equal proportions → z ≈ 0, p ≈ 1.
	z, p := TwoProportionZTest(50, 100, 50, 100)
	if math.Abs(z) > 0.001 {
		t.Errorf("z-score should be ~0, got %.3f", z)
	}
	if math.Abs(p-1.0) > 0.01 {
		t.Errorf("p-value should be ~1, got %.4f", p)
	}
}

func TestTwoProportionZTestZeroN(t *testing.T) {
	z, p := TwoProportionZTest(0, 0, 10, 100)
	if z != 0 || p != 1 {
		t.Errorf("expected z=0, p=1 for zero n, got z=%.3f p=%.4f", z, p)
	}
}

func TestTwoProportionZTestAllSuccess(t *testing.T) {
	// Both groups 100% success → se=0 → z=0, p=1.
	z, p := TwoProportionZTest(100, 100, 100, 100)
	if z != 0 || p != 1 {
		t.Errorf("expected z=0, p=1 for equal 100%%, got z=%.3f p=%.4f", z, p)
	}
}

func TestNormalCDF(t *testing.T) {
	cases := []struct {
		x    float64
		want float64
	}{
		{0, 0.5},
		{-3, 0.00135},
		{3, 0.99865},
		{1.96, 0.975},
	}
	for _, c := range cases {
		got := normalCDF(c.x)
		if math.Abs(got-c.want) > 0.002 {
			t.Errorf("normalCDF(%.2f) = %.5f, want ~%.5f", c.x, got, c.want)
		}
	}
}
