package model

import (
	"math"
	"testing"
)

func TestRelevanceScore(t *testing.T) {
	tests := []struct {
		name           string
		patternOverlap float64
		cosineSim      float64
		want           float64
	}{
		{"zeros", 0, 0, 0},
		{"ones", 1.0, 1.0, 1.0},
		{"pattern_only", 1.0, 0, 0.6},
		{"cosine_only", 0, 1.0, 0.4},
		{"mixed", 0.8, 0.5, 0.6*0.8 + 0.4*0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RelevanceScore(tt.patternOverlap, tt.cosineSim)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("RelevanceScore(%v, %v) = %v, want %v", tt.patternOverlap, tt.cosineSim, got, tt.want)
			}
		})
	}
}
