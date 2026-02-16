// Package extraction implements the 3-stage skill extraction pipeline.
// Stage 1 filters sessions by metadata heuristics, Stage 2 scores novelty
// and rubric quality via embeddings and LLM, and Stage 3 applies an LLM
// critic for final extract/reject verdicts. Extracted skills are persisted
// as SKILL.md files with structured front matter.
package extraction

import (
	"context"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// Stage1Filter performs metadata pre-filtering. Synchronous, cheap, no I/O.
type Stage1Filter interface {
	Filter(session model.SessionRecord) *model.Stage1Result
}

// NoveltyChecker checks whether extracted content is novel enough to persist.
// Optional in PipelineConfig — pipeline works without it.
type NoveltyChecker interface {
	Check(ctx context.Context, content string) (*NoveltyResult, error)
}
