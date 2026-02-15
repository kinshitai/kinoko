package model

import "context"

// SkillCommitter pushes a skill to a git repository.
type SkillCommitter interface {
	CommitSkill(ctx context.Context, libraryID string, skill *SkillRecord, body []byte) (string, error)
}
