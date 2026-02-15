package storage

import (
	"context"

	"github.com/mycelium-dev/mycelium/internal/extraction"
)

// NewSkillQuerier returns an extraction.SkillQuerier backed by the given store.
func NewSkillQuerier(store *SQLiteStore) extraction.SkillQuerier {
	return &storeQuerier{store: store}
}

type storeQuerier struct {
	store *SQLiteStore
}

func (sq *storeQuerier) QueryNearest(ctx context.Context, emb []float32, libraryID string) (*extraction.SkillQueryResult, error) {
	results, err := sq.store.Query(ctx, SkillQuery{
		Embedding:  emb,
		LibraryIDs: []string{libraryID},
		Limit:      1,
	})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &extraction.SkillQueryResult{CosineSim: results[0].CosineSim}, nil
}
