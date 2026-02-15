// Package decay implements skill decay based on half-life degradation.
package decay

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/mycelium-dev/mycelium/internal/extraction"
)

// Config configures the decay system.
type Config struct {
	FoundationalHalfLifeDays int     `yaml:"foundational_half_life_days"` // default: 365
	TacticalHalfLifeDays     int     `yaml:"tactical_half_life_days"`     // default: 90
	ContextualHalfLifeDays   int     `yaml:"contextual_half_life_days"`   // default: 180
	DeprecationThreshold     float64 `yaml:"deprecation_threshold"`       // default: 0.05
	RescueWindowDays         int     `yaml:"rescue_window_days"`          // default: 30
}

// DefaultConfig returns a Config with spec defaults.
func DefaultConfig() Config {
	return Config{
		FoundationalHalfLifeDays: 365,
		TacticalHalfLifeDays:     90,
		ContextualHalfLifeDays:   180,
		DeprecationThreshold:     0.05,
		RescueWindowDays:         30,
	}
}

// SkillReader reads skills for decay processing.
type SkillReader interface {
	ListByDecay(ctx context.Context, libraryID string, limit int) ([]extraction.SkillRecord, error)
}

// SkillWriter writes updated decay scores.
type SkillWriter interface {
	UpdateDecay(ctx context.Context, id string, decayScore float64) error
}

// CycleResult holds counts from a decay cycle.
type CycleResult struct {
	Processed  int
	Demoted    int
	Deprecated int
	Rescued    int
}

// Runner implements DecayRunner.
type Runner struct {
	reader SkillReader
	writer SkillWriter
	cfg    Config
	log    *slog.Logger
	now    func() time.Time // injectable clock for testing
}

// NewRunner creates a decay runner.
func NewRunner(reader SkillReader, writer SkillWriter, cfg Config, log *slog.Logger) *Runner {
	return &Runner{
		reader: reader,
		writer: writer,
		cfg:    cfg,
		log:    log,
		now:    time.Now,
	}
}

// RunCycle performs one decay pass over all skills in a library.
func (r *Runner) RunCycle(ctx context.Context, libraryID string) (*CycleResult, error) {
	skills, err := r.reader.ListByDecay(ctx, libraryID, math.MaxInt32)
	if err != nil {
		return nil, fmt.Errorf("decay: list skills: %w", err)
	}

	result := &CycleResult{Processed: len(skills)}
	now := r.now()

	for i := range skills {
		s := &skills[i]

		oldDecay := s.DecayScore
		halfLife := r.halfLifeDays(s.Category)
		daysSince := now.Sub(s.UpdatedAt).Hours() / 24.0

		newDecay := oldDecay * math.Pow(0.5, daysSince/float64(halfLife))

		// Rescue: recent successful usage boosts decay back.
		if r.shouldRescue(s, now) {
			newDecay = math.Min(1.0, newDecay+0.3)
			result.Rescued++
		}

		// Clamp.
		newDecay = math.Max(0.0, math.Min(1.0, newDecay))

		// Deprecation check.
		if newDecay < r.cfg.DeprecationThreshold {
			newDecay = 0.0
			result.Deprecated++
		} else if newDecay < oldDecay {
			result.Demoted++
		}

		if newDecay != oldDecay {
			if err := r.writer.UpdateDecay(ctx, s.ID, newDecay); err != nil {
				return nil, fmt.Errorf("decay: update %s: %w", s.ID, err)
			}
		}
	}

	r.log.Info("decay cycle complete",
		"library", libraryID,
		"processed", result.Processed,
		"demoted", result.Demoted,
		"deprecated", result.Deprecated,
		"rescued", result.Rescued,
	)

	return result, nil
}

func (r *Runner) halfLifeDays(cat extraction.SkillCategory) int {
	switch cat {
	case extraction.CategoryFoundational:
		return r.cfg.FoundationalHalfLifeDays
	case extraction.CategoryTactical:
		return r.cfg.TacticalHalfLifeDays
	case extraction.CategoryContextual:
		return r.cfg.ContextualHalfLifeDays
	default:
		return r.cfg.ContextualHalfLifeDays
	}
}

func (r *Runner) shouldRescue(s *extraction.SkillRecord, now time.Time) bool {
	if s.LastInjectedAt.IsZero() {
		return false
	}
	daysSinceInjection := now.Sub(s.LastInjectedAt).Hours() / 24.0
	return daysSinceInjection <= float64(r.cfg.RescueWindowDays) && s.SuccessCorrelation > 0
}
