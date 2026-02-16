package model

import "context"

// SkillQueryResult holds the nearest-neighbor result from a skill store query.
type SkillQueryResult struct {
	CosineSim float64
	SkillName string
}

// SkillQuerier finds nearest-neighbor skills by embedding.
type SkillQuerier interface {
	QueryNearest(ctx context.Context, embedding []float32, libraryID string) (*SkillQueryResult, error)
}
