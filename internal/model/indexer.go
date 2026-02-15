package model

import "context"

// SkillIndexer upserts skill metadata and embedding into the discovery index.
type SkillIndexer interface {
	IndexSkill(ctx context.Context, skill *SkillRecord, embedding []float32) error
}
