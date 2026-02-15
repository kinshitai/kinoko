package injection

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mycelium-dev/mycelium/internal/extraction"
	"github.com/mycelium-dev/mycelium/internal/storage"
)

// ABConfig controls the injection A/B test (spec §3.3).
type ABConfig struct {
	Enabled       bool    `yaml:"enabled"`
	ControlRatio  float64 `yaml:"control_ratio"`   // fraction in control group, default 0.1
	MinSampleSize int     `yaml:"min_sample_size"` // min sessions per group before computing results
}

// DefaultABConfig returns a config with spec defaults.
func DefaultABConfig() ABConfig {
	return ABConfig{
		Enabled:       false,
		ControlRatio:  0.1,
		MinSampleSize: 100,
	}
}

// ABGroup identifies the A/B test assignment.
type ABGroup string

const (
	ABGroupTreatment ABGroup = "treatment"
	ABGroupControl   ABGroup = "control"
)

// ABInjector wraps an Injector with A/B test logic.
// Treatment group: normal injection.
// Control group: injection runs but skills are not delivered; events logged with delivered=false.
type ABInjector struct {
	inner       Injector
	eventWriter InjectionEventWriter
	config      ABConfig
	mu          sync.Mutex     // protects randFunc
	randFunc    func() float64 // for testing; guarded by mu
	log         *slog.Logger
}

// NewABInjector wraps inner with A/B testing. If config is not enabled, Inject
// delegates directly to inner (which writes its own events).
// When enabled, ABInjector writes events with A/B group info — ensure the inner
// injector was constructed without an eventWriter to avoid double-writing.
func NewABInjector(inner Injector, eventWriter InjectionEventWriter, config ABConfig, log *slog.Logger) *ABInjector {
	if log == nil {
		log = slog.Default()
	}
	if config.ControlRatio <= 0 || config.ControlRatio >= 1 {
		config.ControlRatio = 0.1
	}
	if config.MinSampleSize <= 0 {
		config.MinSampleSize = 100
	}
	return &ABInjector{
		inner:       inner,
		eventWriter: eventWriter,
		config:      config,
		randFunc:    rand.Float64,
		log:         log.With("component", "ab_injector"),
	}
}

// Inject assigns the session to a group and either delivers or withholds skills.
func (ab *ABInjector) Inject(ctx context.Context, req extraction.InjectionRequest) (*extraction.InjectionResponse, error) {
	if !ab.config.Enabled {
		return ab.inner.Inject(ctx, req)
	}

	group := ab.assignGroup()
	ab.log.Info("ab assignment", "session_id", req.SessionID, "group", group)

	// Always run the real injection to get candidates.
	resp, err := ab.inner.Inject(ctx, req)
	if err != nil {
		return nil, err
	}

	delivered := group == ABGroupTreatment

	// Log A/B events for each skill candidate.
	// NOTE: injection_events are per-skill-per-session (one row per candidate skill).
	// session_outcome is per-session, propagated identically to all events in that session.
	if ab.eventWriter != nil && req.SessionID != "" {
		now := time.Now().UTC()
		for _, sk := range resp.Skills {
			ev := storage.InjectionEventRecord{
				ID:             uuid.Must(uuid.NewV7()).String(),
				SessionID:      req.SessionID,
				SkillID:        sk.SkillID,
				RankPosition:   sk.RankPosition,
				MatchScore:     sk.CompositeScore,
				PatternOverlap: sk.PatternOverlap,
				CosineSim:      sk.CosineSim,
				HistoricalRate: sk.HistoricalRate,
				InjectedAt:     now,
				ABGroup:        string(group),
				Delivered:      delivered,
			}
			if writeErr := ab.eventWriter.WriteInjectionEvent(ctx, ev); writeErr != nil {
				ab.log.Error("failed to write ab injection event", "error", writeErr)
			}
		}
	}

	if !delivered {
		// Control group: return empty skills.
		return &extraction.InjectionResponse{
			Skills:         nil,
			Classification: resp.Classification,
		}, nil
	}

	return resp, nil
}

func (ab *ABInjector) assignGroup() ABGroup {
	ab.mu.Lock()
	r := ab.randFunc()
	ab.mu.Unlock()
	if r < ab.config.ControlRatio {
		return ABGroupControl
	}
	return ABGroupTreatment
}

// SetRandFunc sets the random function (for testing only). Thread-safe.
func (ab *ABInjector) SetRandFunc(f func() float64) {
	ab.mu.Lock()
	ab.randFunc = f
	ab.mu.Unlock()
}
