package storage

import (
	"context"
	"fmt"
	"sort"
)

// SimilarSkill is a skill with its cosine similarity score.
type SimilarSkill struct {
	SkillID   string
	Name      string
	LibraryID string
	Score     float64
	FilePath  string
}

// FindSimilar returns skill embeddings ranked by cosine similarity to the query vector.
// Returns at most limit results with similarity >= minScore.
//
// NOTE: This is a brute-force scan over all embeddings. Adequate for small corpora
// but will not scale. TODO: migrate to pgvector or sqlite-vss for ANN indexing.
func (s *SQLiteStore) FindSimilar(ctx context.Context, queryVec []float32, minScore float64, limit int) ([]SimilarSkill, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT se.skill_id, se.embedding, sk.name, sk.library_id, sk.file_path
		 FROM skill_embeddings se
		 JOIN skills sk ON sk.id = se.skill_id`)
	if err != nil {
		return nil, fmt.Errorf("query skill embeddings: %w", err)
	}
	defer rows.Close()

	var results []SimilarSkill
	for rows.Next() {
		var skillID, name, libraryID, filePath string
		var blob []byte
		if err := rows.Scan(&skillID, &blob, &name, &libraryID, &filePath); err != nil {
			return nil, fmt.Errorf("scan embedding: %w", err)
		}

		vec := bytesToFloat32s(blob)
		sim := cosineSimilarity(queryVec, vec)
		if sim >= minScore {
			results = append(results, SimilarSkill{
				SkillID:   skillID,
				Name:      name,
				LibraryID: libraryID,
				Score:     sim,
				FilePath:  filePath,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
