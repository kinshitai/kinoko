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
	RescueBoost              float64 `yaml:"rescue_boost"`                // default: 0.3
	// RescueWindowDays is an implementation extension not in the original spec.
	// Skills injected within this window with positive success correlation
	// receive a configurable boost (RescueBoost) to their decay score.
	RescueWindowDays int `yaml:"rescue_window_days"` // default: 30
}

// DefaultConfig returns a Config with spec defaults.
func DefaultConfig() Config {
	return Config{
		FoundationalHalfLifeDays: 365,
		TacticalHalfLifeDays:     90,
		ContextualHalfLifeDays:   180,
		DeprecationThreshold:     0.05,
		RescueBoost:              0.3,
		RescueWindowDays:         30,
	}
}

// ValidateConfig returns an error if any half-life is zero or negative.
func ValidateConfig(cfg Config) error {
	if cfg.FoundationalHalfLifeDays <= 0 {
		return fmt.Errorf("decay: foundational half-life must be > 0, got %d", cfg.FoundationalHalfLifeDays)
	}
	if cfg.TacticalHalfLifeDays <= 0 {
		return fmt.Errorf("decay: tactical half-life must be > 0, got %d", cfg.TacticalHalfLifeDays)
	}
	if cfg.ContextualHalfLifeDays <= 0 {
		return fmt.Errorf("decay: contextual half-life must be > 0, got %d", cfg.ContextualHalfLifeDays)
	}
	if cfg.RescueBoost < 0 || cfg.RescueBoost > 1 {
		return fmt.Errorf("decay: rescue boost must be in [0,1], got %f", cfg.RescueBoost)
	}
	return nil
}

// SkillReader reads skills for decay processing.
type SkillReader interface {
	// ListByDecay returns skills ordered by decay score.
	// Pass limit=0 to retrieve all skills (no limit).
	ListByDecay(ctx context.Context, libraryID string, limit int) ([]extraction.SkillRecord, error)
}

// SkillWriter writes updated decay scores.
//
// Contract: UpdateDecay MUST set the record's updated_at to the current time.
// The decay runner uses LastInjectedAt (not UpdatedAt) as the time anchor for
// computing elapsed days, so UpdatedAt changes from UpdateDecay do not affect
// future decay calculations. Implementations must NOT use UpdatedAt as the
// time reference for the next decay cycle.
type SkillWriter interface {
	UpdateDecay(ctx context.Context, id string, decayScore float64) error
}

// DecayCycleResult holds counts from a decay cycle.
type DecayCycleResult struct {
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

// NewRunner creates a decay runner. Returns an error if config is invalid.
func NewRunner(reader SkillReader, writer SkillWriter, cfg Config, log *slog.Logger) (*Runner, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, err
	}
	return &Runner{
		reader: reader,
		writer: writer,
		cfg:    cfg,
		log:    log,
		now:    time.Now,
	}, nil
}

// RunCycle performs one decay pass over all skills in a library.
func (r *Runner) RunCycle(ctx context.Context, libraryID string) (*DecayCycleResult, error) {
	// limit=0 means no limit per SkillReader contract.
	skills, err := r.reader.ListByDecay(ctx, libraryID, 0)
	if err != nil {
		return nil, fmt.Errorf("decay: list skills: %w", err)
	}

	result := &DecayCycleResult{Processed: len(skills)}
	now := r.now()

	for i := range skills {
		s := &skills[i]

		oldDecay := s.DecayScore
		halfLife := r.halfLifeDays(s.Category)

		// Use LastInjectedAt as the time anchor for decay calculation.
		// This avoids double-counting: UpdateDecay updates updated_at,
		// but we measure elapsed time from the last injection event.
		daysSince := now.Sub(s.LastInjectedAt).Hours() / 24.0
		if s.LastInjectedAt.IsZero() {
			// Fallback: skill was never injected, use creation time via UpdatedAt.
			daysSince = now.Sub(s.UpdatedAt).Hours() / 24.0
		}

		var newDecay float64
		// Guard: zero half-life treated as immortal (no decay).
		if halfLife <= 0 {
			newDecay = oldDecay
		} else {
			newDecay = oldDecay * math.Pow(0.5, daysSince/float64(halfLife))
		}

		// Rescue: recent successful usage boosts decay back.
		// This is an implementation extension (not in original spec).
		if r.shouldRescue(s, now) {
			newDecay = math.Min(1.0, newDecay+r.cfg.RescueBoost)
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
		"library_id", libraryID,
		"processed", result.Processed,
		"demoted", result.Demoted,
		"deprecated", result.Deprecated,
		"rescued", result.Rescued,
	)

	return result, nil
}

// SetNow overrides the clock function for testing.
func (r *Runner) SetNow(fn func() time.Time) {
	r.now = fn
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
