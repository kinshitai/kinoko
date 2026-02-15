package extraction

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/kinoko-dev/kinoko/internal/model"
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
	committer  model.SkillCommitter // optional: pushes skills to git
	extractor  string               // pipeline version identifier

	// Stratified sampling counters: maintain ~50/50 extracted vs rejected.
	extractedSamples int
	rejectedSamples  int
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
	Committer  model.SkillCommitter
	Extractor  string
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
	return &Pipeline{
		stage1:     cfg.Stage1,
		stage2:     cfg.Stage2,
		stage3:     cfg.Stage3,
		writer:     cfg.Writer,
		sessions:   cfg.Sessions,
		embedder:   cfg.Embedder,
		reviewer:   cfg.Reviewer,
		committer:  cfg.Committer,
		log:        cfg.Log,
		sampleRate: cfg.SampleRate,
		randIntn:   r,
		extractor:  ext,
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
		return result, nil
	}
	result.Stage2 = s2

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
		return result, nil
	}
	result.Stage3 = s3

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

	// Git push is the only write path. The post-receive hook populates SQLite.
	if p.committer != nil {
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

	p.maybeSample(ctx, session.ID, result)
	return result, nil
}

// updateSessionStatus updates the session record with rejection/error info if sessions are being tracked.
func (p *Pipeline) updateSessionStatus(ctx context.Context, session *model.SessionRecord, result *model.ExtractionResult) {
	if p.sessions == nil {
		return
	}
	session.ExtractionStatus = result.Status
	if result.Status == model.StatusRejected {
		if result.Stage1 != nil && !result.Stage1.Passed {
			session.RejectedAtStage = 1
			session.RejectionReason = result.Stage1.Reason
		} else if result.Stage2 != nil && !result.Stage2.Passed {
			session.RejectedAtStage = 2
			session.RejectionReason = result.Stage2.Reason
		} else if result.Stage3 != nil && !result.Stage3.Passed {
			session.RejectedAtStage = 3
			session.RejectionReason = result.Stage3.CriticReasoning
		}
	}
	if result.Status == model.StatusError {
		session.RejectionReason = result.Error
	}
	if err := p.sessions.UpdateSessionResult(ctx, session); err != nil {
		p.log.Error("failed to update session result", "session_id", session.ID, "error", err)
	}
}

