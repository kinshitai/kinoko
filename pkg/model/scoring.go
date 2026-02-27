package model

const (
	// PatternOverlapWeight is the weight for pattern overlap in relevance scoring.
	PatternOverlapWeight = 0.6
	// CosineSimilarityWeight is the weight for cosine similarity in relevance scoring.
	CosineSimilarityWeight = 0.4
)

// RelevanceScore computes the weighted relevance score from pattern overlap and cosine similarity.
func RelevanceScore(patternOverlap, cosineSim float64) float64 {
	return PatternOverlapWeight*patternOverlap + CosineSimilarityWeight*cosineSim
}
