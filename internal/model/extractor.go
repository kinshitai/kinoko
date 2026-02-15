package model

import "context"

// Extractor runs the 3-stage extraction pipeline on a session.
type Extractor interface {
	Extract(ctx context.Context, session SessionRecord, content []byte) (*ExtractionResult, error)
}
