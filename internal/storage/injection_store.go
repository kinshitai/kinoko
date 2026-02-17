package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/kinoko-dev/kinoko/internal/model"
)

// InjectionEventRecord is an alias for model.InjectionEventRecord.
type InjectionEventRecord = model.InjectionEventRecord

// WriteInjectionEvent inserts a row into injection_events.
func (s *SQLiteStore) WriteInjectionEvent(ctx context.Context, ev InjectionEventRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO injection_events (id, session_id, skill_id, rank_position, match_score, pattern_overlap, cosine_sim, historical_rate, injected_at, ab_group, delivered)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.ID, ev.SessionID, ev.SkillID, ev.RankPosition, ev.MatchScore,
		ev.PatternOverlap, ev.CosineSim, ev.HistoricalRate, ev.InjectedAt, ev.ABGroup, ev.Delivered)
	if err != nil {
		return fmt.Errorf("insert injection event: %w", err)
	}
	return nil
}

// UpdateInjectionOutcome sets session_outcome on injection events for a given session.
func (s *SQLiteStore) UpdateInjectionOutcome(ctx context.Context, sessionID string, outcome string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE injection_events SET session_outcome = ? WHERE session_id = ?`,
		outcome, sessionID)
	if err != nil {
		return fmt.Errorf("update injection outcome for session %s: %w", sessionID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update injection outcome rows affected: %w", err)
	}
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// InsertReviewSample inserts a row into the human_review_samples table.
func (s *SQLiteStore) InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error {
	id := fmt.Sprintf("hrs-%s-%d", sessionID, time.Now().UnixNano())
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO human_review_samples (id, session_id, extraction_result, sampled_at)
		VALUES (?, ?, ?, ?)`,
		id, sessionID, string(resultJSON), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert review sample: %w", err)
	}
	return nil
}
