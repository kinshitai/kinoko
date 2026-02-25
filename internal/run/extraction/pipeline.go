package extraction

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/kinoko-dev/kinoko/internal/run/debug"
	"github.com/kinoko-dev/kinoko/internal/run/sanitize"
	"github.com/kinoko-dev/kinoko/internal/shared/config"
	"github.com/kinoko-dev/kinoko/pkg/model"
)

// Compile-time interface check.
var _ model.Extractor = (*Pipeline)(nil)

// SessionWriter persists and updates session records.
type SessionWriter interface {
	InsertSession(ctx context.Context, session *model.SessionRecord) error
	UpdateSessionResult(ctx context.Context, session *model.SessionRecord) error
}

// HumanReviewWriter writes extraction results selected for human review.
type HumanReviewWriter interface {
	InsertReviewSample(ctx context.Context, sessionID string, resultJSON []byte) error
}

// Pipeline implements model.Extractor by wiring Stage1 → Stage2 → Stage3 → git commit.
type Pipeline struct {
	stage1     Stage1Filter
	stage2     Stage2Scorer
	stage3     Stage3Critic
	sessions   SessionWriter
	reviewer   HumanReviewWriter
	log        *slog.Logger
	sampleRate float64 // 0.0–1.0, e.g. 0.01 for 1%
	randIntn   RandIntn
	novelty    NoveltyChecker          // optional: checks skill novelty before commit
	committer  model.SkillCommitter    // required: commits skills to git
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
	Sessions   SessionWriter
	Reviewer   HumanReviewWriter
	Log        *slog.Logger
	SampleRate float64
	RandIntn   RandIntn
	Novelty    NoveltyChecker
	Committer  model.SkillCommitter
	Scanner    *sanitize.Scanner
	Extractor  string
	Tracer     *debug.Tracer
	ExtCfg     config.ExtractionConfig
}

// NewPipeline creates a Pipeline. If RandIntn is nil, crypto/rand is used.
// Returns an error if required dependencies (Stage1, Stage2, Stage3, Committer, Log) are nil.
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
	if cfg.Committer == nil {
		return nil, fmt.Errorf("pipeline: Committer is required")
	}
	r := cfg.RandIntn
	if r == nil {
		r = cryptoRandIntn
	}
	ext := cfg.Extractor
	if ext == "" {
		ext = "pipeline-v1"
	}
	return &Pipeline{
		stage1:     cfg.Stage1,
		stage2:     cfg.Stage2,
		stage3:     cfg.Stage3,
		sessions:   cfg.Sessions,
		reviewer:   cfg.Reviewer,
		novelty:    cfg.Novelty,
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

// extractionRun holds mutable state for a single Extract() invocation.
type extractionRun struct {
	ctx     context.Context
	session model.SessionRecord
	content []byte
	start   time.Time
	result  *model.ExtractionResult
	trace   *debug.RunTrace
	s1Ms    int64
	s2Ms    int64
	s3Ms    int64
}

// Extract runs the full extraction pipeline on a session.
func (p *Pipeline) Extract(ctx context.Context, session model.SessionRecord, content []byte) (*model.ExtractionResult, error) {
	run := &extractionRun{
		ctx:     ctx,
		session: session,
		content: content,
		start:   time.Now(),
		result: &model.ExtractionResult{
			SessionID:   session.ID,
			ProcessedAt: time.Now(),
		},
		trace: p.tracer.StartRun(),
	}
	run.trace.WriteSession(content)

	p.log.Info("pipeline start", "session_id", session.ID)
	p.prepare(run)

	if done := p.filter(run); done {
		return run.result, nil
	}
	if done := p.score(run); done {
		return run.result, nil
	}
	if done := p.critique(run); done {
		return run.result, nil
	}
	return p.publish(run)
}

// prepare persists the session before extraction begins.
func (p *Pipeline) prepare(run *extractionRun) {
	if p.sessions != nil {
		run.session.ExtractionStatus = model.StatusPending
		if err := p.sessions.InsertSession(run.ctx, &run.session); err != nil {
			p.log.Error("failed to insert session", "session_id", run.session.ID, "error", err)
		}
	}
}

// filter runs Stage 1 filtering. Returns true if the pipeline should stop.
func (p *Pipeline) filter(run *extractionRun) bool {
	p.log.Info("stage1 entry", "session_id", run.session.ID)
	s1Start := time.Now()
	s1 := p.stage1.Filter(run.session)
	run.s1Ms = time.Since(s1Start).Milliseconds()
	run.result.Stage1 = s1

	run.trace.WriteStage("stage1-filter", debug.Stage1Trace{
		Passed: s1.Passed,
		Filters: map[string]debug.FilterTrace{
			"duration_minutes": {
				Value:     run.session.DurationMinutes,
				Threshold: []float64{p.extCfg.MinDurationMinutes, p.extCfg.MaxDurationMinutes},
				Passed:    s1.DurationOK,
			},
			"tool_call_count": {
				Value:     run.session.ToolCallCount,
				Threshold: p.extCfg.MinToolCalls,
				Passed:    s1.ToolCallCountOK,
			},
			"error_rate": {
				Value:     run.session.ErrorRate,
				Threshold: p.extCfg.MaxErrorRate,
				Passed:    s1.ErrorRateOK,
			},
			"has_successful_exec": {
				Value:     run.session.HasSuccessfulExec,
				Threshold: true,
				Passed:    s1.HasSuccessExec,
			},
		},
		DurationMs: run.s1Ms,
	})

	if !s1.Passed {
		run.result.Status = model.StatusRejected
		run.result.DurationMs = time.Since(run.start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", run.session.ID,
			"stage", 1,
			"reason", s1.Reason,
			"stage1_ms", run.s1Ms,
			"total_ms", run.result.DurationMs,
		)
		p.updateSessionStatus(run.ctx, &run.session, run.result)
		p.maybeSample(run.ctx, run.session.ID, run.result)
		rejAt := "stage1"
		p.writeTraceSummary(run.trace, run.result, &rejAt, 0, run.s1Ms, 0, 0)
		return true
	}
	p.log.Info("stage1 pass", "session_id", run.session.ID, "stage1_ms", run.s1Ms)
	return false
}

// score runs Stage 2 scoring. Returns true if the pipeline should stop.
func (p *Pipeline) score(run *extractionRun) bool {
	p.log.Info("stage2 entry", "session_id", run.session.ID)
	s2Start := time.Now()
	s2, err := p.stage2.Score(run.ctx, run.session, run.content)
	run.s2Ms = time.Since(s2Start).Milliseconds()
	if err != nil {
		run.result.Status = model.StatusError
		run.result.Error = fmt.Sprintf("stage2 [session=%s]: %v", run.session.ID, err)
		run.result.DurationMs = time.Since(run.start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", run.session.ID,
			"stage", 2,
			"error", err,
			"stage2_ms", run.s2Ms,
			"total_ms", run.result.DurationMs,
		)
		p.updateSessionStatus(run.ctx, &run.session, run.result)
		p.maybeSample(run.ctx, run.session.ID, run.result)
		rejAt := "stage2-error"
		p.writeTraceSummary(run.trace, run.result, &rejAt, 0, run.s1Ms, run.s2Ms, 0)
		return true
	}
	run.result.Stage2 = s2

	run.trace.WriteStage("stage2-scoring", debug.Stage2Trace{
		Passed: s2.Passed,
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
		RubricThreshold: 3.0,
		DurationMs:      run.s2Ms,
	})

	if !s2.Passed {
		run.result.Status = model.StatusRejected
		run.result.DurationMs = time.Since(run.start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", run.session.ID,
			"stage", 2,
			"reason", s2.Reason,
			"stage2_ms", run.s2Ms,
			"total_ms", run.result.DurationMs,
		)
		p.updateSessionStatus(run.ctx, &run.session, run.result)
		p.maybeSample(run.ctx, run.session.ID, run.result)
		rejAt := "stage2"
		p.writeTraceSummary(run.trace, run.result, &rejAt, 0, run.s1Ms, run.s2Ms, 0)
		return true
	}
	p.log.Info("stage2 pass", "session_id", run.session.ID, "stage2_ms", run.s2Ms)
	return false
}

// critique runs Stage 3 evaluation. Returns true if the pipeline should stop.
func (p *Pipeline) critique(run *extractionRun) bool {
	s2 := run.result.Stage2
	p.log.Info("stage3 entry", "session_id", run.session.ID)
	s3Start := time.Now()
	s3, err := p.stage3.Evaluate(run.ctx, run.session, run.content, s2)
	run.s3Ms = time.Since(s3Start).Milliseconds()
	if err != nil {
		run.result.Status = model.StatusError
		run.result.Error = fmt.Sprintf("stage3 [session=%s]: %v", run.session.ID, err)
		run.result.DurationMs = time.Since(run.start).Milliseconds()
		p.log.Error("pipeline error",
			"session_id", run.session.ID,
			"stage", 3,
			"error", err,
			"stage3_ms", run.s3Ms,
			"total_ms", run.result.DurationMs,
		)
		p.updateSessionStatus(run.ctx, &run.session, run.result)
		p.maybeSample(run.ctx, run.session.ID, run.result)
		rejAt := "stage3-error"
		p.writeTraceSummary(run.trace, run.result, &rejAt, 0, run.s1Ms, run.s2Ms, run.s3Ms)
		return true
	}
	run.result.Stage3 = s3

	run.trace.WriteStage("stage3-critic", debug.Stage3Trace{
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
		DurationMs: run.s3Ms,
	})

	if !s3.Passed {
		run.result.Status = model.StatusRejected
		run.result.DurationMs = time.Since(run.start).Milliseconds()
		p.log.Info("pipeline reject",
			"session_id", run.session.ID,
			"stage", 3,
			"reason", s3.CriticReasoning,
			"stage3_ms", run.s3Ms,
			"total_ms", run.result.DurationMs,
		)
		p.updateSessionStatus(run.ctx, &run.session, run.result)
		p.maybeSample(run.ctx, run.session.ID, run.result)
		rejAt := "stage3"
		p.writeTraceSummary(run.trace, run.result, &rejAt, 0, run.s1Ms, run.s2Ms, run.s3Ms)
		return true
	}
	p.log.Info("stage3 pass", "session_id", run.session.ID, "stage3_ms", run.s3Ms)
	return false
}

// publish builds the skill record, checks novelty, and commits to git.
func (p *Pipeline) publish(run *extractionRun) (*model.ExtractionResult, error) {
	s2 := run.result.Stage2
	s3 := run.result.Stage3

	// Use LLM-generated SKILL.md if available; fall back to template.
	skillName := skillNameFromClassification(s2.ClassifiedPatterns, s2.ClassifiedCategory)
	skillCategory := s2.ClassifiedCategory
	skillPatterns := s2.ClassifiedPatterns
	skillVersion := 1
	skillDescription := ""
	var generatedBody []byte

	if s3.SkillMD != "" {
		parsedName, parsedVersion, parsedCategory, parsedTags, parsedDescription, parseErr := ParseGeneratedSkillMD(s3.SkillMD)
		if parseErr != nil {
			p.log.Warn("failed to parse LLM-generated SKILL.md, falling back to template",
				"session_id", run.session.ID, "error", parseErr)
		} else {
			skillName = parsedName
			skillDescription = parsedDescription
			skillVersion = parsedVersion
			if parsedCategory != "" {
				skillCategory = model.SkillCategory(parsedCategory)
			}
			if len(parsedTags) > 0 {
				skillPatterns = parsedTags
			}
			generatedBody = []byte(s3.SkillMD)
		}
	}

	skillID := uuid.Must(uuid.NewV7()).String()
	now := time.Now()

	skill := &model.SkillRecord{
		ID:              skillID,
		Name:            skillName,
		Description:     skillDescription,
		Version:         skillVersion,
		LibraryID:       run.session.LibraryID,
		Category:        skillCategory,
		Patterns:        skillPatterns,
		Quality:         s3.RefinedScores,
		SourceSessionID: run.session.ID,
		ExtractedBy:     p.extractor,
		FilePath:        fmt.Sprintf("skills/%s/v%d/SKILL.md", skillName, skillVersion),
		DecayScore:      1.0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	var body []byte
	if generatedBody != nil {
		body = generatedBody
	} else {
		body = buildSkillMD(skill, s3, run.content)
	}

	run.trace.WriteSkill(skillName, body)

	if p.scanner != nil && p.scanner.HasSecrets(string(body)) {
		p.log.Warn("credentials detected in generated skill, redacting",
			"session_id", run.session.ID,
			"skill_name", skillName,
		)
		body = []byte(p.scanner.Redact(string(body)))
	}

	novel := true
	if p.novelty != nil {
		noveltyResult, noveltyErr := p.novelty.Check(run.ctx, string(body))
		if noveltyErr != nil {
			p.log.Warn("novelty check failed, treating as novel", "session_id", run.session.ID, "error", noveltyErr)
		} else if !noveltyResult.Novel {
			novel = false
			similarName := ""
			if len(noveltyResult.Similar) > 0 {
				similarName = noveltyResult.Similar[0].Name
			}
			p.log.Info("skill not novel, skipping commit",
				"session_id", run.session.ID,
				"skill_name", skillName,
				"score", noveltyResult.Score,
				"similar_to", similarName,
			)
		}
	}

	if novel {
		commitStart := time.Now()
		commitHash, commitErr := p.committer.CommitSkill(run.ctx, run.session.LibraryID, skill, body)
		commitMs := time.Since(commitStart).Milliseconds()
		if commitErr != nil {
			run.result.Status = model.StatusError
			run.result.Error = fmt.Sprintf("git commit [session=%s]: %v", run.session.ID, commitErr)
			run.result.DurationMs = time.Since(run.start).Milliseconds()
			p.log.Error("git commit failed",
				"session_id", run.session.ID,
				"skill_id", skillID,
				"error", commitErr,
				"commit_ms", commitMs,
				"total_ms", run.result.DurationMs,
			)
			p.updateSessionStatus(run.ctx, &run.session, run.result)
			p.maybeSample(run.ctx, run.session.ID, run.result)
			p.writeTraceSummary(run.trace, run.result, nil, 0, run.s1Ms, run.s2Ms, run.s3Ms)
			return run.result, nil
		}
		run.result.CommitHash = commitHash
		p.log.Info("skill committed to git", "session_id", run.session.ID, "skill_id", skillID, "hash", commitHash, "commit_ms", commitMs)
	}

	run.result.Status = model.StatusExtracted
	run.result.Skill = skill
	run.result.DurationMs = time.Since(run.start).Milliseconds()

	p.log.Info("pipeline extracted",
		"session_id", run.session.ID,
		"skill_id", skillID,
		"skill_name", skillName,
		"total_ms", run.result.DurationMs,
	)

	if p.sessions != nil {
		run.session.ExtractionStatus = model.StatusExtracted
		run.session.ExtractedSkillID = skillID
		if err := p.sessions.UpdateSessionResult(run.ctx, &run.session); err != nil {
			p.log.Error("failed to update session result", "session_id", run.session.ID, "error", err)
		}
	}

	p.writeTraceSummary(run.trace, run.result, nil, 1, run.s1Ms, run.s2Ms, run.s3Ms)
	p.maybeSample(run.ctx, run.session.ID, run.result)
	return run.result, nil
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
