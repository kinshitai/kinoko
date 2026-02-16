package storage

import (
	"context"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// NewSkillQuerier returns a model.SkillQuerier backed by the given store.
func NewSkillQuerier(store *SQLiteStore) model.SkillQuerier {
	return &storeQuerier{store: store}
}

type storeQuerier struct {
	store *SQLiteStore
}

func (sq *storeQuerier) QueryNearest(ctx context.Context, emb []float32, libraryID string) (*model.SkillQueryResult, error) {
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
	return &model.SkillQueryResult{CosineSim: results[0].CosineSim}, nil
}
