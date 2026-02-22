package extraction

import (
	"context"

	"github.com/kinoko-dev/kinoko/pkg/model"
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
