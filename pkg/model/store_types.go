package model

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors for callers to check with errors.Is.
var (
	ErrNotFound  = errors.New("skill not found")
	ErrDuplicate = errors.New("duplicate skill")
)

// SkillStore persists and retrieves skills.
type SkillStore interface {
	Put(ctx context.Context, skill *SkillRecord, body []byte) error
	Get(ctx context.Context, id string) (*SkillRecord, error)
	GetLatestByName(ctx context.Context, name string, libraryID string) (*SkillRecord, error)
	Query(ctx context.Context, q SkillQuery) ([]ScoredSkill, error)
	UpdateUsage(ctx context.Context, id string, outcome string) error
	UpdateDecay(ctx context.Context, id string, decayScore float64) error
	ListByDecay(ctx context.Context, libraryID string, limit int) ([]SkillRecord, error)
}

// SkillQuery defines query parameters for skill search.
type SkillQuery struct {
	Patterns   []string
	Embedding  []float32
	LibraryIDs []string
	MinQuality float64
	MinDecay   float64
	Limit      int
}

// ScoredSkill is a skill with match scores.
type ScoredSkill struct {
	Skill          SkillRecord
	PatternOverlap float64
	CosineSim      float64
	HistoricalRate float64
	CompositeScore float64
}

// InjectionEventRecord maps to the injection_events table.
type InjectionEventRecord struct {
	ID             string
	SessionID      string
	SkillID        string
	RankPosition   int
	MatchScore     float64
	PatternOverlap float64
	CosineSim      float64
	HistoricalRate float64
	InjectedAt     time.Time
	ABGroup        string // "treatment", "control", or "" (no A/B test)
	Delivered      bool   // false for control group sessions
}

// SimilarSkill is a skill with its cosine similarity score.
type SimilarSkill struct {
	SkillID   string
	Name      string
	LibraryID string
	Score     float64
	FilePath  string
}
