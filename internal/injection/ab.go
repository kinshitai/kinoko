package injection

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

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
	randFunc    func() float64 // for testing
	log         *slog.Logger
}

// NewABInjector wraps inner with A/B testing. If config is not enabled, Inject
// delegates directly to inner with treatment logging.
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
	if ab.eventWriter != nil && req.SessionID != "" {
		now := time.Now().UTC()
		for i, sk := range resp.Skills {
			ev := storage.InjectionEventRecord{
				ID:             fmt.Sprintf("%s-%s-%d-ab", req.SessionID, sk.SkillID, i),
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
	if ab.randFunc() < ab.config.ControlRatio {
		return ABGroupControl
	}
	return ABGroupTreatment
}
