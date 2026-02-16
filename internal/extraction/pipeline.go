package extraction

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/kinoko-dev/kinoko/internal/config"
	"github.com/kinoko-dev/kinoko/internal/debug"
	"github.com/kinoko-dev/kinoko/internal/model"
	"github.com/kinoko-dev/kinoko/internal/sanitize"
)

// Compile-time interface check.
var _ model.Extractor = (*Pipeline)(nil)

// SkillWriter persists a skill record and its SKILL.md body.
type SkillWriter interface {
	Put(ctx context.Context, skill *model.SkillRecord, body []byte) error
}

// SessionWriter persists and updates session records.
type SessionWriter interface {
	InsertSession(ctx context.Context, session *model.SessionRecord) error
	UpdateSessionResult(ctx context.Context, session *model.SessionRecord) error
}

// HumanReviewWriter writes extraction results selected for human review.
type HumanReviewWriter interface {
	InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error
}

// SkillEmbedder computes an embedding for skill content.
type SkillEmbedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Pipeline implements model.Extractor by wiring Stage1 → Stage2 → Stage3 → SkillWriter.
type Pipeline struct {
	stage1     Stage1Filter
	stage2     Stage2Scorer
	stage3     Stage3Critic
	writer     SkillWriter
	sessions   SessionWriter
	embedder   SkillEmbedder
	reviewer   HumanReviewWriter
	log        *slog.Logger
	sampleRate float64 // 0.0–1.0, e.g. 0.01 for 1%
	randIntn   RandIntn
	novelty    NoveltyChecker          // optional: checks skill novelty before push
	pusher     SkillPusher             // optional: pushes skills to git (Phase C)
	committer  model.SkillCommitter    // optional: pushes skills to git
	scanner    *sanitize.Scanner       // optional: credential scanner
	tracer     *debug.Tracer           // optional: pipeline debug tracing
	extractor  string                  // pipeline version identifier
	extCfg     config.ExtractionConfig // extraction config for trace thresholds

	// Stratified sampling counters: maintain ~50/50 extracted vs rejected.
	// Accessed atomically for concurrency safety.
	extractedSamples atomic.Int64
	rejectedSamples  atomic.Int64
}

// PipelineConfig holds constructor parameters for Pipeline.
type PipelineConfig struct {
	Stage1     Stage1Filter
	Stage2     Stage2Scorer
	Stage3     Stage3Critic
	Writer     SkillWriter
	Sessions   SessionWriter
	Embedder   SkillEmbedder
	Reviewer   HumanReviewWriter
	Log        *slog.Logger
	SampleRate float64
	RandIntn   RandIntn
	Novelty    NoveltyChecker
	Pusher     SkillPusher
	Committer  model.SkillCommitter
	Scanner    *sanitize.Scanner
	Extractor  string
	Tracer     *debug.Tracer
	ExtCfg     config.ExtractionConfig
}

// NewPipeline creates a Pipeline. If RandIntn is nil, crypto/rand is used.
// Returns an error if required dependencies (Stage1, Stage2, Stage3, Writer, Log) are nil.
func NewPipeline(cfg PipelineConfig) (*Pipeline, error) {
	if cfg.Stage1 == nil {
		return nil, fmt.Errorf("pipeline: Stage1 is required")
	}
	if cfg.Stage2 == nil {
		return nil, fmt.Errorf("pipeline: Stage2 is required")
	}
	if cfg.Stage3 == nil {
		return nil, fmt.Errorf("pipeline: Stage3 is required")
	}
	if cfg.Log == nil {
		return nil, fmt.Errorf("pipeline: Log is required")
	}
	r := cfg.RandIntn
	if r == nil {
		r = cryptoRandIntn
	}
	ext := cfg.Extractor
	if ext == "" {
		ext = "pipeline-v1"
	}
	// P1-1: Warn if committer is nil — skills will be marked extracted without git persistence.
	if cfg.Committer == nil {
		cfg.Log.Warn("pipeline created without committer: extracted skills will not be persisted to git")
	}

	return &Pipeline{
		stage1:     cfg.Stage1,
		stage2:     cfg.Stage2,
		stage3:     cfg.Stage3,
		writer:     cfg.Writer,
		sessions:   cfg.Sessions,
		embedder:   cfg.Embedder,
		reviewer:   cfg.Reviewer,
		novelty:    cfg.Novelty,
		pusher:     cfg.Pusher,
		committer:  cfg.Committer,
		scanner:    cfg.Scanner,
		tracer:     cfg.Tracer,
		log:        cfg.Log,
		sampleRate: cfg.SampleRate,
		randIntn:   r,
		extractor:  ext,
		extCfg:     cfg.ExtCfg,
	}, nil
}

// SetCommitter sets the git committer. Safe to call before Extract is used.
func (p *Pipeline) SetCommitter(c model.SkillCommitter) {
	p.committer = c
}

// Extract runs the full extraction pipeline on a session.
func (p *Pipeline) Extract(ctx context.Context, session model.SessionRecord, content []byte) (*model.ExtractionResult, error) {
	start := time.Now()
	result := &model.ExtractionResult{
		SessionID:   session.ID,
		ProcessedAt: start,
	}

	// Debug tracing — all writes are best-effort, nil-safe.
	trace := p.tracer.StartRun()
	trace.WriteSession(content)

	p.log.Info("pipeline start", "session_id", session.ID)

	// Persist session before extraction begins.
	if p.sessions != nil {
		session.ExtractionStatus = model.StatusPending
		if err := p.sessions.InsertSession(ctx, &session); err != nil {
			p.log.Error("failed to insert session", "session_id", session.ID, "error", err)
			// Non-fatal: continue extraction even if session persistence fails.
		}
	}

	// Stage 1
	p.log.Info("stage1 entry", "session_id", session.ID)
	s1Start := time.Now()
	s1 := p.stage1.Filter(session)
	s1Ms := time.Since(s1Start).Milliseconds()
	result.Stage1 = s1

	// Debug: stage 1 trace with real filter names and config thresholds.
	trace.WriteStage("stage1-filter", debug.Stage1Trace{
		Passed: s1.Passed,
		Filters: map[string]debug.FilterTrace{
			"duration_minutes": {
				Value:     session.DurationMinutes,
				Threshold: []float64{p.extCfg.MinDurationMinutes, p.extCfg.MaxDurationMinutes},
				Passed:    s1.DurationOK,
			},
			"tool_call_count": {
				Value:     session.ToolCallCount,
				Threshold: p.extCfg.MinToolCalls,
				Passed:    s1.ToolCallCountOK,
			},
			"error_rate": {
				Value:     session.ErrorRate,
				Threshold: p.extCfg.MaxErrorRate,
				Passed:    s1.ErrorRateOK,
			},
			"has_successful_exec": {
				Value:     session.HasSuccessfulExec,
				Threshold: true,
				Passed:    s1.HasSuccessExec,
			},
		},
		DurationMs: s1Ms,
	})

	if !s1.Passed {
		result.Status = model.StatusRejected
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", session.ID,
			"stage", 1,
			"reason", s1.Reason,
			"stage1_ms", s1Ms,
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
		p.maybeSample(ctx, session.ID, result)
		rejAt := "stage1"
		p.writeTraceSummary(trace, result, &rejAt, 0, s1Ms, 0, 0)
		return result, nil
	}
	p.log.Info("stage1 pass", "session_id", session.ID, "stage1_ms", s1Ms)

	// Stage 2
	p.log.Info("stage2 entry", "session_id", session.ID)
	s2Start := time.Now()
	s2, err := p.stage2.Score(ctx, session, content)
	s2Ms := time.Since(s2Start).Milliseconds()
	if err != nil {
		result.Status = model.StatusError
		result.Error = fmt.Sprintf("stage2 [session=%s]: %v", session.ID, err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", 2,
			"error", err,
			"stage2_ms", s2Ms,
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
		p.maybeSample(ctx, session.ID, result)
		rejAt := "stage2-error"
		p.writeTraceSummary(trace, result, &rejAt, 0, s1Ms, s2Ms, 0)
		return result, nil
	}
	result.Stage2 = s2

	// Debug: stage 2 trace with individual rubric scores and embedding details.
	trace.WriteStage("stage2-scoring", debug.Stage2Trace{
		Passed: s2.Passed,
		EmbeddingNovelty: &debug.EmbeddingTrace{
			Distance:     s2.EmbeddingDistance,
			NearestSkill: s2.NearestSkillName,
			Threshold:    p.extCfg.NoveltyMinDistance,
		},
		RubricScores: map[string]float64{
			"problem_specificity":    float64(s2.RubricScores.ProblemSpecificity),
			"solution_completeness":  float64(s2.RubricScores.SolutionCompleteness),
			"context_portability":    float64(s2.RubricScores.ContextPortability),
			"reasoning_transparency": float64(s2.RubricScores.ReasoningTransparency),
			"technical_accuracy":     float64(s2.RubricScores.TechnicalAccuracy),
			"verification_evidence":  float64(s2.RubricScores.VerificationEvidence),
			"innovation_level":       float64(s2.RubricScores.InnovationLevel),
		},
		RubricAggregate: s2.RubricScores.CompositeScore,
		RubricThreshold: 3.0, // MinimumViable requires each of problem_specificity, solution_completeness, technical_accuracy >= 3
		DurationMs:      s2Ms,
	})

	if !s2.Passed {
		result.Status = model.StatusRejected
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", session.ID,
			"stage", 2,
			"reason", s2.Reason,
			"stage2_ms", s2Ms,
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
		p.maybeSample(ctx, session.ID, result)
		rejAt := "stage2"
		p.writeTraceSummary(trace, result, &rejAt, 0, s1Ms, s2Ms, 0)
		return result, nil
	}
	p.log.Info("stage2 pass", "session_id", session.ID, "stage2_ms", s2Ms)

	// Stage 3
	p.log.Info("stage3 entry", "session_id", session.ID)
	s3Start := time.Now()
	s3, err := p.stage3.Evaluate(ctx, session, content, s2)
	s3Ms := time.Since(s3Start).Milliseconds()
	if err != nil {
		result.Status = model.StatusError
		result.Error = fmt.Sprintf("stage3 [session=%s]: %v", session.ID, err)
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", session.ID,
			"stage", 3,
			"error", err,
			"stage3_ms", s3Ms,
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
		p.maybeSample(ctx, session.ID, result)
		rejAt := "stage3-error"
		p.writeTraceSummary(trace, result, &rejAt, 0, s1Ms, s2Ms, s3Ms)
		return result, nil
	}
	result.Stage3 = s3

	// Debug: stage 3 trace with real values from Stage3Result.
	trace.WriteStage("stage3-critic", debug.Stage3Trace{
		Passed:                 s3.Passed,
		Verdict:                s3.CriticVerdict,
		Confidence:             s3.RefinedScores.CriticConfidence,
		Reasoning:              s3.CriticReasoning,
		ContradictionsDetected: s3.ContradictsBestPractices,
		Retries:                s3.Retries,
		CircuitBreakerState:    s3.CircuitBreakerState,
		Meta: &debug.LLMMeta{
			Model:     s3.ModelName,
			TokensOut: s3.TokensUsed,
			LatencyMs: s3.LatencyMs,
		},
		DurationMs: s3Ms,
	})

	if !s3.Passed {
		result.Status = model.StatusRejected
		result.DurationMs = time.Since(start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", session.ID,
			"stage", 3,
			"reason", s3.CriticReasoning,
			"stage3_ms", s3Ms,
			"total_ms", result.DurationMs,
		)
		p.updateSessionStatus(ctx, &session, result)
		p.maybeSample(ctx, session.ID, result)
		rejAt := "stage3"
		p.writeTraceSummary(trace, result, &rejAt, 0, s1Ms, s2Ms, s3Ms)
		return result, nil
	}
	p.log.Info("stage3 pass", "session_id", session.ID, "stage3_ms", s3Ms)

	// Build skill and persist
	skillName := skillNameFromClassification(s2.ClassifiedPatterns, s2.ClassifiedCategory)
	skillID := uuid.Must(uuid.NewV7()).String()
	now := time.Now()

	skill := &model.SkillRecord{
		ID:              skillID,
		Name:            skillName,
		Version:         1,
		LibraryID:       session.LibraryID,
		Category:        s2.ClassifiedCategory,
		Patterns:        s2.ClassifiedPatterns,
		Quality:         s3.RefinedScores,
		SourceSessionID: session.ID,
		ExtractedBy:     p.extractor,
		FilePath:        fmt.Sprintf("skills/%s/v1/SKILL.md", skillName),
		DecayScore:      1.0, // New skills start fully active; 0.0 would hide them from injection queries.
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	// Compute embedding for the skill content so injection can use cosine similarity.
	if p.embedder != nil {
		emb, embErr := p.embedder.Embed(ctx, string(content))
		if embErr != nil {
			p.log.Warn("failed to compute skill embedding, storing without", "session_id", session.ID, "error", embErr)
		} else {
			skill.Embedding = emb
		}
	}

	body := buildSkillMD(skill, s3, content)

	// Debug: write extracted skill
	trace.WriteSkill(skillName, body)

	// Credential scanning: redact secrets before git push.
	if p.scanner != nil && p.scanner.HasSecrets(string(body)) {
		p.log.Warn("credentials detected in generated skill, redacting",
			"session_id", session.ID,
			"skill_name", skillName,
		)
		body = []byte(p.scanner.Redact(string(body)))
	}

	// Novelty check: if configured, skip non-novel skills.
	novel := true
	if p.novelty != nil {
		noveltyResult, noveltyErr := p.novelty.Check(ctx, string(body))
		if noveltyErr != nil {
			p.log.Warn("novelty check failed, treating as novel", "session_id", session.ID, "error", noveltyErr)
		} else if !noveltyResult.Novel {
			novel = false
			similarName := ""
			if len(noveltyResult.Similar) > 0 {
				similarName = noveltyResult.Similar[0].Name
			}
			p.log.Info("skill not novel, skipping push",
				"session_id", session.ID,
				"skill_name", skillName,
				"score", noveltyResult.Score,
				"similar_to", similarName,
			)
		}
	}

	// Phase C pusher: push to git if novel.
	if p.pusher != nil && novel {
		if pushErr := p.pusher.Push(ctx, skillName, session.LibraryID, body); pushErr != nil {
			p.log.Error("skill push failed", "session_id", session.ID, "skill_name", skillName, "error", pushErr)
			// Non-fatal: continue to local persistence.
		}
	}

	// Git push is the only write path. The post-receive hook populates SQLite.
	if p.committer != nil && novel {
		commitStart := time.Now()
		commitHash, commitErr := p.committer.CommitSkill(ctx, session.LibraryID, skill, body)
		commitMs := time.Since(commitStart).Milliseconds()
		if commitErr != nil {
			result.Status = model.StatusError
			result.Error = fmt.Sprintf("git commit [session=%s]: %v", session.ID, commitErr)
			result.DurationMs = time.Since(start).Milliseconds()
			p.log.Error("git commit failed",
				"session_id", session.ID,
				"skill_id", skillID,
				"error", commitErr,
				"commit_ms", commitMs,
				"total_ms", result.DurationMs,
			)
			p.updateSessionStatus(ctx, &session, result)
			p.maybeSample(ctx, session.ID, result)
			p.writeTraceSummary(trace, result, nil, 0, s1Ms, s2Ms, s3Ms)
			return result, nil
		}
		result.CommitHash = commitHash
		p.log.Info("skill committed to git", "session_id", session.ID, "skill_id", skillID, "hash", commitHash, "commit_ms", commitMs)
	}

	result.Status = model.StatusExtracted
	result.Skill = skill
	result.DurationMs = time.Since(start).Milliseconds()

	p.log.Info("pipeline extracted",
		"session_id", session.ID,
		"skill_id", skillID,
		"skill_name", skillName,
		"total_ms", result.DurationMs,
	)

	// Update session with extraction result.
	if p.sessions != nil {
		session.ExtractionStatus = model.StatusExtracted
		session.ExtractedSkillID = skillID
		if err := p.sessions.UpdateSessionResult(ctx, &session); err != nil {
			p.log.Error("failed to update session result", "session_id", session.ID, "error", err)
		}
	}

	p.writeTraceSummary(trace, result, nil, 1, s1Ms, s2Ms, s3Ms)
	p.maybeSample(ctx, session.ID, result)
	return result, nil
}

// updateSessionStatus updates the session record with rejection/error info if sessions are being tracked.
func (p *Pipeline) updateSessionStatus(ctx context.Context, session *model.SessionRecord, result *model.ExtractionResult) {
	if p.sessions == nil {
		return
	}
	session.ExtractionStatus = result.Status
	switch result.Status {
	case model.StatusRejected:
		switch {
		case result.Stage1 != nil && !result.Stage1.Passed:
			session.RejectedAtStage = 1
			session.RejectionReason = result.Stage1.Reason
		case result.Stage2 != nil && !result.Stage2.Passed:
			session.RejectedAtStage = 2
			session.RejectionReason = result.Stage2.Reason
		case result.Stage3 != nil && !result.Stage3.Passed:
			session.RejectedAtStage = 3
			session.RejectionReason = result.Stage3.CriticReasoning
		}
	case model.StatusError:
		session.RejectionReason = result.Error
	}
	if err := p.sessions.UpdateSessionResult(ctx, session); err != nil {
		p.log.Error("failed to update session result", "session_id", session.ID, "error", err)
	}
}

// writeTraceSummary writes the debug summary. All args are best-effort; trace may be nil.
func (p *Pipeline) writeTraceSummary(trace *debug.RunTrace, result *model.ExtractionResult, rejectedAt *string, skillsExtracted int, s1Ms, s2Ms, s3Ms int64) {
	if trace == nil {
		return
	}
	now := time.Now()
	stages := map[string]debug.StageSum{
		"stage1": {Passed: result.Stage1 != nil && result.Stage1.Passed, DurationMs: s1Ms},
	}
	if result.Stage2 != nil {
		stages["stage2"] = debug.StageSum{Passed: result.Stage2.Passed, DurationMs: s2Ms}
	}
	if result.Stage3 != nil {
		stages["stage3"] = debug.StageSum{Passed: result.Stage3.Passed, DurationMs: s3Ms}
	}

	var cost *debug.CostEstimate
	if result.Stage3 != nil && result.Stage3.TokensUsed > 0 {
		cost = &debug.CostEstimate{TokensOut: result.Stage3.TokensUsed}
	}

	trace.WriteSummary(debug.Summary{
		TraceID:         trace.TraceID,
		SessionFile:     "session.log",
		StartedAt:       trace.Started(),
		FinishedAt:      now,
		DurationMs:      result.DurationMs,
		Result:          string(result.Status),
		RejectedAt:      rejectedAt,
		SkillsExtracted: skillsExtracted,
		Stages:          stages,
		CostEstimate:    cost,
	})
}
